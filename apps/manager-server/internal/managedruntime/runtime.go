package managedruntime

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/gateway"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/model"
)

const RuntimeHandoffOperationEnv = "CPA_MANAGER_RUNTIME_HANDOFF_OPERATION_ID"

const (
	runtimeHandoffConfirmationTimeout = 90 * time.Second
	runtimeShutdownTimeout            = 40 * time.Second
)

type RuntimeTarget struct {
	Binary      string
	OperationID string
}

type HandoffError struct {
	Binary      string
	OperationID string
}

func (e *HandoffError) Error() string {
	return "handoff CPAMP runtime to " + e.Binary
}

func Run(ctx context.Context, cfg Config) (runErr error) {
	deployment := model.DefaultDeploymentState(cfg.DeploymentMode, cfg.PanelBasePath, cfg.PanelBasePathSource)
	state, err := OpenStateStore(cfg.StatePath, deployment)
	if err != nil {
		return err
	}
	snapshot, err := reconcileConfiguredPanelBasePath(cfg, state)
	if err != nil {
		return err
	}
	snapshot, err = prepareRuntimeComponents(cfg, state)
	if err != nil {
		return err
	}
	snapshot, err = reconcileInstallerManagedRuntimeConfig(cfg, state, snapshot)
	if err != nil {
		return err
	}
	activeBinary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve active runtime binary: %w", err)
	}
	handoffOperation, confirmingHandoff, err := resolvePendingRuntimeHandoff(
		snapshot,
		os.Getenv(RuntimeHandoffOperationEnv),
		activeBinary,
	)
	if err != nil {
		if confirmingHandoff {
			return errors.Join(
				err,
				recoverPendingRuntimeHandoffState(state, cfg, handoffOperation.ID, err),
			)
		}
		return err
	}
	handoffConfirmed := false
	if confirmingHandoff {
		defer func() {
			if runErr == nil || handoffConfirmed {
				return
			}
			runErr = errors.Join(
				runErr,
				recoverPendingRuntimeHandoffState(state, cfg, handoffOperation.ID, runErr),
			)
		}()
	}
	cfg.DeploymentMode = snapshot.Deployment.Mode
	cfg.PanelBasePath = snapshot.Deployment.PanelBasePath
	if snapshot.CPAUpstreamURL == "" {
		snapshot, err = state.UpdateCPAUpstream(cfg.CPAURL)
		if err != nil {
			return err
		}
	} else {
		cfg.CPAURL = snapshot.CPAUpstreamURL
	}
	if cfg.DeploymentMode == model.DeploymentModeIntegrated && cfg.CPABinary == "" {
		cfg.CPABinary = snapshot.Components["cpa"].BinaryPath
		if cfg.CPABinary == "" {
			cfg.CPABinary = findCPABinary(cfg.ManagerBinary)
		}
	}
	secrets, err := EnsureAssets(cfg)
	if err != nil {
		return err
	}
	router, err := gateway.New(
		cfg.ManagerURL,
		cfg.CPAURL,
		cfg.ControlURL,
		snapshot.Deployment.PanelBasePath,
		snapshot.Deployment.RetiredPanelPaths,
	)
	if err != nil {
		return err
	}
	gatewayServer, err := gateway.Listen(cfg.GatewayAddrs, router)
	if err != nil {
		return err
	}
	controlListener, err := net.Listen("tcp", cfg.ControlAddr)
	if err != nil {
		_ = gatewayServer.Close()
		return fmt.Errorf("listen on runtime control address %s: %w", cfg.ControlAddr, err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	supervisor := NewProcessSupervisor(state, os.Stderr)
	controller := NewRuntimeController(runCtx, cfg, state, router, secrets, supervisor)
	handoff := make(chan runtimeHandoff, 1)
	controller.SetHandoffChannel(handoff)
	result := make(chan error, 2)
	go func() { result <- gatewayServer.Serve(runCtx) }()
	go func() {
		result <- serveControl(runCtx, controlListener, NewControlHandler(controller, secrets.RuntimeKey))
	}()

	controller.StartManager()
	if cfg.DeploymentMode == model.DeploymentModeIntegrated {
		if err := controller.StartCPA(); err != nil {
			return errors.Join(err, stopRuntime(runCtx, cancel, controller, result, 2))
		}
	}

	log.Printf("cpamp runtime gateway listening on %v", cfg.GatewayAddrs)
	if confirmingHandoff {
		ready := make(chan error, 1)
		go func() {
			ready <- controller.waitRuntimeHandoffReady(
				runCtx,
				handoffOperation,
				runtimeHandoffConfirmationTimeout,
			)
		}()
		select {
		case <-ctx.Done():
			shutdownErr := stopRuntime(runCtx, cancel, controller, result, 2)
			return errors.Join(
				fmt.Errorf("runtime stopped before handoff confirmation: %w", ctx.Err()),
				shutdownErr,
			)
		case listenerErr := <-result:
			shutdownErr := stopRuntime(runCtx, cancel, controller, result, 1)
			if listenerErr == nil || errors.Is(listenerErr, context.Canceled) {
				listenerErr = errors.New("runtime listener stopped before handoff confirmation")
			}
			return errors.Join(listenerErr, shutdownErr)
		case readinessErr := <-ready:
			if readinessErr != nil {
				return errors.Join(
					readinessErr,
					stopRuntime(runCtx, cancel, controller, result, 2),
				)
			}
		}
		if _, err := state.CompletePendingHandoff(handoffOperation.ID); err != nil {
			return errors.Join(
				fmt.Errorf("confirm runtime handoff: %w", err),
				stopRuntime(runCtx, cancel, controller, result, 2),
			)
		}
		handoffConfirmed = true
	}
	select {
	case <-ctx.Done():
		return stopRuntime(runCtx, cancel, controller, result, 2)
	case request := <-handoff:
		if err := stopRuntime(runCtx, cancel, controller, result, 2); err != nil {
			return fmt.Errorf("stop runtime for handoff: %w", err)
		}
		return &HandoffError{Binary: request.Binary, OperationID: request.OperationID}
	case err := <-result:
		shutdownErr := stopRuntime(runCtx, cancel, controller, result, 1)
		if err == nil || errors.Is(err, context.Canceled) {
			return shutdownErr
		}
		return errors.Join(err, shutdownErr)
	}
}

type runtimeProcessWaiter interface {
	WaitProcesses(context.Context) error
}

func stopRuntime(
	runCtx context.Context,
	cancel context.CancelFunc,
	processes runtimeProcessWaiter,
	listeners <-chan error,
	pendingListeners int,
) error {
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), runtimeShutdownTimeout)
	defer shutdownCancel()
	var result error
	for range pendingListeners {
		select {
		case err := <-listeners:
			if err != nil && !errors.Is(err, context.Canceled) {
				result = errors.Join(result, err)
			}
		case <-shutdownCtx.Done():
			return errors.Join(result, fmt.Errorf("stop runtime listeners: %w", shutdownCtx.Err()))
		}
	}
	if err := processes.WaitProcesses(shutdownCtx); err != nil {
		result = errors.Join(result, fmt.Errorf("stop runtime processes: %w", err))
	}
	if runCtx.Err() == nil {
		result = errors.Join(result, errors.New("runtime cancellation did not propagate"))
	}
	return result
}

func CurrentRuntimeTarget(cfg Config) (RuntimeTarget, error) {
	if strings.TrimSpace(os.Getenv("CPA_MANAGER_RUNTIME_DELEGATED")) != "" {
		return RuntimeTarget{}, nil
	}
	if _, err := os.Stat(cfg.StatePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RuntimeTarget{}, nil
		}
		return RuntimeTarget{}, err
	}
	deployment := model.DefaultDeploymentState(cfg.DeploymentMode, cfg.PanelBasePath, cfg.PanelBasePathSource)
	state, err := OpenStateStore(cfg.StatePath, deployment)
	if err != nil {
		return RuntimeTarget{}, err
	}
	snapshot, err := prepareRuntimeComponents(cfg, state)
	if err != nil {
		return RuntimeTarget{}, err
	}
	candidate := strings.TrimSpace(snapshot.Components["cpamp"].BinaryPath)
	if candidate == "" {
		return RuntimeTarget{}, nil
	}
	current, err := filepath.Abs(cfg.ManagerBinary)
	if err != nil {
		return RuntimeTarget{}, err
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return RuntimeTarget{}, err
	}
	if runtimeBinaryPathsEqual(candidate, current) {
		return RuntimeTarget{}, nil
	}
	pending, awaitingHandoff, err := resolvePendingRuntimeHandoff(snapshot, "", candidate)
	if err != nil {
		return RuntimeTarget{}, err
	}
	if err := validateReplacementRuntimeBinary(candidate); err != nil {
		if awaitingHandoff {
			if recoveryErr := recoverPendingRuntimeHandoffState(
				state,
				cfg,
				pending.ID,
				err,
			); recoveryErr != nil {
				return RuntimeTarget{}, errors.Join(err, recoveryErr)
			}
			return RuntimeTarget{}, nil
		}
		return RuntimeTarget{}, err
	}
	target := RuntimeTarget{Binary: candidate}
	if awaitingHandoff {
		target.OperationID = pending.ID
	}
	return target, nil
}

func CurrentRuntimeBinary(cfg Config) (string, error) {
	target, err := CurrentRuntimeTarget(cfg)
	return target.Binary, err
}

func RecoverPendingHandoff(cfg Config, operationID string, cause error) (bool, error) {
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return false, nil
	}
	deployment := model.DefaultDeploymentState(cfg.DeploymentMode, cfg.PanelBasePath, cfg.PanelBasePathSource)
	state, err := OpenStateStore(cfg.StatePath, deployment)
	if err != nil {
		return false, err
	}
	operation, ok := state.Operation(operationID)
	if !ok || operation.Status != runtimeUpdateStatusHandoffPending {
		return false, nil
	}
	if err := recoverPendingRuntimeHandoffState(state, cfg, operationID, cause); err != nil {
		return false, err
	}
	return true, nil
}

func HandoffFallbackTarget(cfg Config, operationID string) (RuntimeTarget, error) {
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return RuntimeTarget{}, nil
	}
	deployment := model.DefaultDeploymentState(cfg.DeploymentMode, cfg.PanelBasePath, cfg.PanelBasePathSource)
	state, err := OpenStateStore(cfg.StatePath, deployment)
	if err != nil {
		return RuntimeTarget{}, err
	}
	operation, ok := state.Operation(operationID)
	if !ok || (operation.Status != runtimeUpdateStatusRolledBack && operation.Status != runtimeUpdateStatusFailed) {
		return RuntimeTarget{}, nil
	}
	candidate := strings.TrimSpace(state.Snapshot().Components["cpamp"].BinaryPath)
	if candidate == "" {
		return RuntimeTarget{}, errors.New("recovered CPAMP runtime binary is unavailable")
	}
	activeBinary, err := os.Executable()
	if err != nil {
		return RuntimeTarget{}, fmt.Errorf("resolve active runtime binary: %w", err)
	}
	if runtimeBinaryPathsEqual(candidate, activeBinary) {
		return RuntimeTarget{}, nil
	}
	if err := validateReplacementRuntimeBinary(candidate); err != nil {
		return RuntimeTarget{}, err
	}
	return RuntimeTarget{Binary: candidate}, nil
}

func recoverPendingRuntimeHandoffState(
	state *StateStore,
	cfg Config,
	operationID string,
	cause error,
) error {
	if _, err := state.FailPendingHandoff(operationID, cause); err != nil {
		return fmt.Errorf("recover pending runtime handoff: %w", err)
	}
	if err := syncCurrentComponentPointers(cfg.ComponentDir, state.Snapshot().Components); err != nil {
		return fmt.Errorf("sync recovered runtime component pointers: %w", err)
	}
	return nil
}

func resolvePendingRuntimeHandoff(
	snapshot State,
	requestedID string,
	activeBinary string,
) (UpdateOperation, bool, error) {
	requestedID = strings.TrimSpace(requestedID)
	activeBinary = strings.TrimSpace(activeBinary)
	if requestedID != "" {
		operation, ok := snapshot.Operations[requestedID]
		if ok && operation.Status == runtimeUpdateStatusHandoffPending {
			operation.ID = requestedID
			if err := validatePendingRuntimeHandoff(snapshot, operation, activeBinary); err != nil {
				return operation, true, err
			}
			return operation, true, nil
		}
	}

	var matched UpdateOperation
	for id, operation := range snapshot.Operations {
		if operation.Status != runtimeUpdateStatusHandoffPending {
			continue
		}
		operation.ID = id
		if err := validatePendingRuntimeHandoff(snapshot, operation, activeBinary); err != nil {
			continue
		}
		if matched.ID != "" {
			return matched, true, errors.New("multiple runtime updates are awaiting handoff confirmation")
		}
		matched = operation
	}
	return matched, matched.ID != "", nil
}

func validatePendingRuntimeHandoff(snapshot State, operation UpdateOperation, activeBinary string) error {
	if !containsComponent(operation.Components, "cpamp") {
		return fmt.Errorf("runtime update %s does not include CPAMP", operation.ID)
	}
	component := snapshot.Components["cpamp"]
	if targetVersion := strings.TrimSpace(operation.TargetVersions["cpamp"]); targetVersion != "" &&
		component.Version != targetVersion {
		return fmt.Errorf(
			"runtime update %s targets CPAMP %s but state points to %s",
			operation.ID,
			targetVersion,
			component.Version,
		)
	}
	if !runtimeBinaryPathsEqual(activeBinary, component.BinaryPath) {
		return fmt.Errorf(
			"runtime update %s expects binary %s but active binary is %s",
			operation.ID,
			component.BinaryPath,
			activeBinary,
		)
	}
	return nil
}

func validateReplacementRuntimeBinary(binary string) error {
	info, err := os.Stat(binary)
	if err != nil {
		return fmt.Errorf("inspect replacement runtime binary %s: %w", binary, err)
	}
	if info.IsDir() {
		return fmt.Errorf("replacement runtime binary %s is a directory", binary)
	}
	if goruntime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("replacement runtime binary %s is not executable", binary)
	}
	return nil
}

func runtimeBinaryPathsEqual(left string, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return false
	}
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	if leftInfo, err := os.Stat(leftAbs); err == nil {
		if rightInfo, rightErr := os.Stat(rightAbs); rightErr == nil && os.SameFile(leftInfo, rightInfo) {
			return true
		}
	}
	leftAbs = filepath.Clean(leftAbs)
	rightAbs = filepath.Clean(rightAbs)
	if goruntime.GOOS == "windows" {
		return strings.EqualFold(leftAbs, rightAbs)
	}
	return leftAbs == rightAbs
}

func prepareRuntimeComponents(cfg Config, state *StateStore) (State, error) {
	snapshot, err := reconcilePackagedRuntimeComponents(cfg, state)
	if err != nil {
		return State{}, err
	}
	if err := syncCurrentComponentPointers(cfg.ComponentDir, snapshot.Components); err != nil {
		return State{}, fmt.Errorf("sync runtime component pointers: %w", err)
	}
	return snapshot, nil
}

func reconcileInstallerManagedRuntimeConfig(cfg Config, state *StateStore, snapshot State) (State, error) {
	if cfg.DeploymentMode != model.DeploymentModeInstallerManaged || !cfg.CPAUpstreamEnvSet {
		return snapshot, nil
	}
	if snapshot.SlimTransition != nil {
		return snapshot, nil
	}
	if snapshot.Deployment.Mode != model.DeploymentModeInstallerManaged {
		deployment, err := state.UpdateDeployment(func(deployment model.DeploymentState) model.DeploymentState {
			runtimeState := model.DefaultDeploymentState(
				model.DeploymentModeInstallerManaged,
				deployment.PanelBasePath,
				deployment.PanelBasePathSource,
			)
			deployment.Mode = runtimeState.Mode
			deployment.RuntimeManaged = runtimeState.RuntimeManaged
			deployment.CPAMPUpdatesManaged = runtimeState.CPAMPUpdatesManaged
			deployment.CPAUpdatesManaged = runtimeState.CPAUpdatesManaged
			return deployment
		})
		if err != nil {
			return State{}, fmt.Errorf("reconcile installer-managed deployment: %w", err)
		}
		snapshot.Deployment = deployment
	}
	configuredURL := strings.TrimSpace(cfg.CPAURL)
	if configuredURL == "" || snapshot.CPAUpstreamURL == configuredURL {
		return snapshot, nil
	}
	updated, err := state.UpdateCPAUpstream(configuredURL)
	if err != nil {
		return State{}, fmt.Errorf("reconcile installer-managed CPA upstream: %w", err)
	}
	return updated, nil
}

func reconcilePackagedRuntimeComponents(cfg Config, state *StateStore) (State, error) {
	snapshot := state.Snapshot()
	packagedRuntime, active, err := activePackagedRuntimeBinary(cfg.ManagerBinary)
	if err != nil {
		return State{}, err
	}
	if !active {
		return snapshot, nil
	}
	locations := map[string]componentLocation{}
	if location, ok, err := newerPackagedComponentLocation(
		"cpamp",
		cfg.CPAMPVersion,
		packagedRuntime,
		snapshot.Components["cpamp"].Version,
	); err != nil {
		return State{}, err
	} else if ok {
		locations["cpamp"] = location
	}
	if snapshot.Deployment.Mode == model.DeploymentModeIntegrated &&
		packagedComponentsShareDirectory(packagedRuntime, cfg.CPABinary) {
		if location, ok, err := newerPackagedComponentLocation(
			"cpa",
			cfg.CPAVersion,
			cfg.CPABinary,
			snapshot.Components["cpa"].Version,
		); err != nil {
			return State{}, err
		} else if ok {
			locations["cpa"] = location
		}
	}
	return state.reconcileComponentLocations(locations)
}

func activePackagedRuntimeBinary(configured string) (string, bool, error) {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return "", false, nil
	}
	executable, err := os.Executable()
	if err != nil {
		return "", false, fmt.Errorf("resolve packaged CPAMP binary: %w", err)
	}
	configured, err = filepath.Abs(configured)
	if err != nil {
		return "", false, fmt.Errorf("resolve configured CPAMP binary: %w", err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return "", false, fmt.Errorf("resolve current CPAMP binary: %w", err)
	}
	if !runtimeBinaryPathsEqual(configured, executable) {
		return "", false, nil
	}
	return executable, true, nil
}

func packagedComponentsShareDirectory(managerBinary string, cpaBinary string) bool {
	managerBinary = strings.TrimSpace(managerBinary)
	cpaBinary = strings.TrimSpace(cpaBinary)
	if managerBinary == "" || cpaBinary == "" {
		return false
	}
	return runtimeBinaryPathsEqual(filepath.Dir(managerBinary), filepath.Dir(cpaBinary))
}

func newerPackagedComponentLocation(
	name string,
	packagedVersion string,
	packagedBinary string,
	managedVersion string,
) (componentLocation, bool, error) {
	comparison, comparable := compareReleaseVersions(packagedVersion, managedVersion)
	if !comparable || comparison <= 0 {
		return componentLocation{}, false, nil
	}
	packagedBinary = strings.TrimSpace(packagedBinary)
	if packagedBinary == "" {
		return componentLocation{}, false, fmt.Errorf("packaged %s binary is missing", name)
	}
	info, err := os.Stat(packagedBinary)
	if err != nil {
		return componentLocation{}, false, fmt.Errorf("inspect packaged %s binary %s: %w", name, packagedBinary, err)
	}
	if info.IsDir() {
		return componentLocation{}, false, fmt.Errorf("packaged %s binary %s is a directory", name, packagedBinary)
	}
	return componentLocation{
		Version:    strings.TrimSpace(packagedVersion),
		BinaryPath: packagedBinary,
	}, true, nil
}

func reconcileConfiguredPanelBasePath(cfg Config, state *StateStore) (State, error) {
	snapshot := state.Snapshot()
	source := cfg.PanelBasePathSource
	value := snapshot.Deployment.PanelBasePath
	if model.PanelBasePathEnvironmentManaged(source) {
		value = cfg.PanelBasePath
	} else if snapshot.Deployment.PanelBasePathSource == model.PanelBasePathSourceRuntime {
		return snapshot, nil
	} else {
		source = model.PanelBasePathSourceRuntime
	}
	if _, err := state.ReconcilePanelBasePath(value, source); err != nil {
		return State{}, err
	}
	return state.Snapshot(), nil
}

func serveControl(ctx context.Context, listener net.Listener, handler http.Handler) error {
	server := &http.Server{
		Addr:              listener.Addr().String(),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	result := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		result <- err
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown runtime control server: %w", err)
		}
		return nil
	case err := <-result:
		return err
	}
}
