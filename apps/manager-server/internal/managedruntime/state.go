package managedruntime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/model"
)

const runtimeStateSchemaVersion = 3

const (
	slimTransitionProvision = "provision"
	slimTransitionExisting  = "existing"
)

const (
	runtimeUpdateStatusQueued         = "queued"
	runtimeUpdateStatusRunning        = "running"
	runtimeUpdateStatusHandoffPending = "handoff_pending"
	runtimeUpdateStatusRollingBack    = "rolling_back"
	runtimeUpdateStatusSucceeded      = "succeeded"
	runtimeUpdateStatusFailed         = "failed"
	runtimeUpdateStatusRolledBack     = "rolled_back"
)

func runtimeUpdateInProgress(status string) bool {
	switch status {
	case runtimeUpdateStatusQueued,
		runtimeUpdateStatusRunning,
		runtimeUpdateStatusHandoffPending,
		runtimeUpdateStatusRollingBack:
		return true
	default:
		return false
	}
}

type ComponentState struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	PID         int    `json:"pid,omitempty"`
	Version     string `json:"version,omitempty"`
	BinaryPath  string `json:"binaryPath,omitempty"`
	Restarts    int    `json:"restarts"`
	LastError   string `json:"lastError,omitempty"`
	StartedAtMS int64  `json:"startedAtMs,omitempty"`
	UpdatedAtMS int64  `json:"updatedAtMs"`
}

type UpdateOperation struct {
	ID                 string                           `json:"id"`
	Kind               string                           `json:"kind"`
	Status             string                           `json:"status"`
	Progress           int                              `json:"progress"`
	Message            string                           `json:"message,omitempty"`
	Components         []string                         `json:"components"`
	CurrentVersions    map[string]string                `json:"currentVersions,omitempty"`
	CurrentBinaries    map[string]string                `json:"currentBinaries,omitempty"`
	TargetVersions     map[string]string                `json:"targetVersions,omitempty"`
	ReleaseURLs        map[string]string                `json:"releaseUrls,omitempty"`
	ReleaseNotes       map[string]string                `json:"releaseNotes,omitempty"`
	Results            map[string]ComponentUpdateResult `json:"results,omitempty"`
	Error              string                           `json:"error,omitempty"`
	RollbackAttempted  bool                             `json:"rollbackAttempted,omitempty"`
	RollbackSuccessful bool                             `json:"rollbackSuccessful,omitempty"`
	StartedAtMS        int64                            `json:"startedAtMs"`
	UpdatedAtMS        int64                            `json:"updatedAtMs"`
	CompletedAtMS      int64                            `json:"completedAtMs,omitempty"`
	OperationTokenHash string                           `json:"operationTokenHash,omitempty"`
}

type ComponentUpdateResult struct {
	Name            string `json:"name"`
	FromVersion     string `json:"fromVersion,omitempty"`
	ToVersion       string `json:"toVersion,omitempty"`
	Status          string `json:"status"`
	Error           string `json:"error,omitempty"`
	RollbackVersion string `json:"rollbackVersion,omitempty"`
}

type SlimTransition struct {
	Kind                   string                `json:"kind"`
	PreviousDeployment     model.DeploymentState `json:"previousDeployment"`
	PreviousCPAUpstreamURL string                `json:"previousCpaUpstreamUrl,omitempty"`
	PreviousCPAComponent   *ComponentState       `json:"previousCpaComponent,omitempty"`
	PreviousCPABinary      string                `json:"previousCpaBinary,omitempty"`
	PreviousCPAStarted     bool                  `json:"previousCpaStarted"`
	TargetCPAUpstreamURL   string                `json:"targetCpaUpstreamUrl,omitempty"`
	Installed              *InstalledComponent   `json:"installed,omitempty"`
	StartedAtMS            int64                 `json:"startedAtMs"`
}

type State struct {
	SchemaVersion     int                        `json:"schemaVersion"`
	Deployment        model.DeploymentState      `json:"deployment"`
	CPAUpstreamURL    string                     `json:"cpaUpstreamUrl,omitempty"`
	Components        map[string]ComponentState  `json:"components"`
	SlimTransition    *SlimTransition            `json:"slimTransition,omitempty"`
	Operations        map[string]UpdateOperation `json:"operations,omitempty"`
	LatestOperationID string                     `json:"latestOperationId,omitempty"`
	UpdatedAtMS       int64                      `json:"updatedAtMs"`
}

type StateStore struct {
	path  string
	mu    sync.RWMutex
	state State
}

type componentLocation struct {
	Version    string
	BinaryPath string
}

func OpenStateStore(path string, deployment model.DeploymentState) (*StateStore, error) {
	store := &StateStore{path: path}
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &store.state); err != nil {
			return nil, fmt.Errorf("parse runtime state: %w", err)
		}
		if err := store.migrate(); err != nil {
			return nil, err
		}
		return store, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read runtime state: %w", err)
	}
	store.state = State{
		SchemaVersion: runtimeStateSchemaVersion,
		Deployment:    deployment,
		Components:    map[string]ComponentState{},
		Operations:    map[string]UpdateOperation{},
	}
	if err := store.persistLocked(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *StateStore) SaveOperation(operation UpdateOperation) error {
	operation.ID = strings.TrimSpace(operation.ID)
	if operation.ID == "" {
		return errors.New("runtime operation ID is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneState(s.state)
	if s.state.Operations == nil {
		s.state.Operations = map[string]UpdateOperation{}
	}
	previousOperation := s.state.Operations[operation.ID]
	now := nextOperationUpdatedAtMS(previousOperation.UpdatedAtMS)
	if operation.StartedAtMS == 0 {
		operation.StartedAtMS = now
	}
	operation.UpdatedAtMS = now
	s.state.Operations[operation.ID] = cloneOperation(operation)
	s.state.LatestOperationID = operation.ID
	s.pruneOperationsLocked(32)
	return s.persistMutationLocked(previous)
}

func (s *StateStore) UpdateOperation(id string, update func(UpdateOperation) UpdateOperation) (UpdateOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneState(s.state)
	operation, ok := s.state.Operations[id]
	if !ok {
		return UpdateOperation{}, os.ErrNotExist
	}
	operation = update(cloneOperation(operation))
	operation.ID = id
	operation.UpdatedAtMS = nextOperationUpdatedAtMS(previous.Operations[id].UpdatedAtMS)
	s.state.Operations[id] = cloneOperation(operation)
	s.state.LatestOperationID = id
	if err := s.persistMutationLocked(previous); err != nil {
		return UpdateOperation{}, err
	}
	return cloneOperation(operation), nil
}

func (s *StateStore) CompletePendingHandoff(id string) (UpdateOperation, error) {
	id = strings.TrimSpace(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneState(s.state)
	operation, ok := s.state.Operations[id]
	if !ok {
		return UpdateOperation{}, os.ErrNotExist
	}
	if operation.Status != runtimeUpdateStatusHandoffPending {
		return UpdateOperation{}, fmt.Errorf(
			"runtime update %s is %s, not awaiting handoff confirmation",
			id,
			operation.Status,
		)
	}
	now := nextOperationUpdatedAtMS(operation.UpdatedAtMS)
	operation.Status = runtimeUpdateStatusSucceeded
	operation.Progress = 100
	operation.Message = "update completed"
	operation.Error = ""
	operation.CompletedAtMS = now
	operation.UpdatedAtMS = now
	s.state.Operations[id] = cloneOperation(operation)
	s.state.LatestOperationID = id
	if err := s.persistMutationLocked(previous); err != nil {
		return UpdateOperation{}, err
	}
	return cloneOperation(operation), nil
}

func (s *StateStore) FailPendingHandoff(id string, cause error) (UpdateOperation, error) {
	id = strings.TrimSpace(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneState(s.state)
	operation, ok := s.state.Operations[id]
	if !ok {
		return UpdateOperation{}, os.ErrNotExist
	}
	if operation.Status != runtimeUpdateStatusHandoffPending {
		return cloneOperation(operation), nil
	}
	now := nextOperationUpdatedAtMS(operation.UpdatedAtMS)
	operation = s.recoverInterruptedOperation(operation, now)
	recoveryError := operation.Error
	operation.Error = "updated runtime failed before handoff confirmation"
	if cause != nil && strings.TrimSpace(cause.Error()) != "" {
		operation.Error += ": " + strings.TrimSpace(cause.Error())
	}
	if rollbackIndex := strings.Index(recoveryError, "; rollback failed:"); rollbackIndex >= 0 {
		operation.Error += recoveryError[rollbackIndex:]
	}
	if operation.Status == runtimeUpdateStatusRolledBack {
		operation.Message = "interrupted update rolled back during runtime startup"
	} else {
		operation.Message = "update failed"
	}
	s.state.Operations[id] = cloneOperation(operation)
	s.state.LatestOperationID = id
	if err := s.persistMutationLocked(previous); err != nil {
		return UpdateOperation{}, err
	}
	return cloneOperation(operation), nil
}

func nextOperationUpdatedAtMS(previous int64) int64 {
	now := time.Now().UnixMilli()
	if now <= previous {
		return previous + 1
	}
	return now
}

func (s *StateStore) Operation(id string) (UpdateOperation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	operation, ok := s.state.Operations[id]
	return cloneOperation(operation), ok
}

func (s *StateStore) LatestOperation() (UpdateOperation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	operation, ok := s.state.Operations[s.state.LatestOperationID]
	return cloneOperation(operation), ok
}

func (s *StateStore) Snapshot() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneState(s.state)
}

func (s *StateStore) UpdateComponent(name string, update func(ComponentState) ComponentState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneState(s.state)
	component := s.state.Components[name]
	component.Name = name
	component = update(component)
	component.Name = name
	component.UpdatedAtMS = time.Now().UnixMilli()
	s.state.Components[name] = component
	return s.persistMutationLocked(previous)
}

func (s *StateStore) reconcileComponentLocations(locations map[string]componentLocation) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneState(s.state)
	changed := false
	now := time.Now().UnixMilli()
	for _, name := range []string{"cpamp", "cpa"} {
		location, ok := locations[name]
		if !ok {
			continue
		}
		version := strings.TrimSpace(location.Version)
		binaryPath := strings.TrimSpace(location.BinaryPath)
		if version == "" || binaryPath == "" {
			continue
		}
		component := s.state.Components[name]
		if component.Version == version && component.BinaryPath == binaryPath {
			continue
		}
		component.Name = name
		component.Version = version
		component.BinaryPath = binaryPath
		component.UpdatedAtMS = now
		s.state.Components[name] = component
		changed = true
	}
	if !changed {
		return cloneState(s.state), nil
	}
	if err := s.persistMutationLocked(previous); err != nil {
		return State{}, err
	}
	return cloneState(s.state), nil
}

func (s *StateStore) UpdatePanelBasePath(value string) (model.DeploymentState, error) {
	return s.updatePanelBasePath(value, model.PanelBasePathSourceRuntime, false)
}

func (s *StateStore) ReconcilePanelBasePath(value string, source string) (model.DeploymentState, error) {
	if !model.PanelBasePathEnvironmentManaged(source) {
		source = model.PanelBasePathSourceRuntime
	}
	return s.updatePanelBasePath(value, source, true)
}

func (s *StateStore) updatePanelBasePath(value string, source string, allowEnvironmentOverride bool) (model.DeploymentState, error) {
	normalized, err := model.NormalizePanelBasePath(value)
	if err != nil {
		return model.DeploymentState{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !allowEnvironmentOverride && model.PanelBasePathEnvironmentManaged(s.state.Deployment.PanelBasePathSource) {
		return model.DeploymentState{}, errors.New("panel base path is managed by the environment")
	}
	if s.state.Deployment.PanelBasePath == normalized && s.state.Deployment.PanelBasePathSource == source {
		return s.state.Deployment, nil
	}
	previousState := cloneState(s.state)
	previous := s.state.Deployment.PanelBasePath
	s.state.Deployment.RetiredPanelPaths = slices.DeleteFunc(
		s.state.Deployment.RetiredPanelPaths,
		func(candidate string) bool { return candidate == normalized },
	)
	if previous != "" && previous != normalized && !slices.Contains(s.state.Deployment.RetiredPanelPaths, previous) {
		s.state.Deployment.RetiredPanelPaths = append(s.state.Deployment.RetiredPanelPaths, previous)
		if len(s.state.Deployment.RetiredPanelPaths) > 32 {
			s.state.Deployment.RetiredPanelPaths = s.state.Deployment.RetiredPanelPaths[len(s.state.Deployment.RetiredPanelPaths)-32:]
		}
	}
	s.state.Deployment.PanelBasePath = normalized
	s.state.Deployment.PanelBasePathSource = source
	s.state.Deployment.UpdatedAtMS = time.Now().UnixMilli()
	if err := s.persistMutationLocked(previousState); err != nil {
		return model.DeploymentState{}, err
	}
	return s.state.Deployment, nil
}

func (s *StateStore) UpdateCPAUpstream(value string) (State, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return State{}, errors.New("CPA upstream URL is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneState(s.state)
	s.state.CPAUpstreamURL = value
	if err := s.persistMutationLocked(previous); err != nil {
		return State{}, err
	}
	return cloneState(s.state), nil
}

func (s *StateStore) UpdateDeployment(update func(model.DeploymentState) model.DeploymentState) (model.DeploymentState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneState(s.state)
	s.state.Deployment = update(s.state.Deployment)
	s.state.Deployment.SchemaVersion = 1
	s.state.Deployment.UpdatedAtMS = time.Now().UnixMilli()
	if err := s.persistMutationLocked(previous); err != nil {
		return model.DeploymentState{}, err
	}
	return s.state.Deployment, nil
}

func (s *StateStore) BeginSlimTransition(
	kind string,
	targetCPAUpstreamURL string,
	previousCPABinary string,
	previousCPAStarted bool,
	installed *InstalledComponent,
) (SlimTransition, error) {
	if kind != slimTransitionProvision && kind != slimTransitionExisting {
		return SlimTransition{}, errors.New("Slim runtime transition kind is invalid")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.SlimTransition != nil {
		return SlimTransition{}, errors.New("a Slim runtime transition is already pending")
	}
	previous := cloneState(s.state)
	transition := SlimTransition{
		Kind:                   kind,
		PreviousDeployment:     cloneDeploymentState(s.state.Deployment),
		PreviousCPAUpstreamURL: s.state.CPAUpstreamURL,
		PreviousCPABinary:      strings.TrimSpace(previousCPABinary),
		PreviousCPAStarted:     previousCPAStarted,
		TargetCPAUpstreamURL:   strings.TrimSpace(targetCPAUpstreamURL),
		StartedAtMS:            time.Now().UnixMilli(),
	}
	if component, ok := s.state.Components["cpa"]; ok {
		componentCopy := component
		transition.PreviousCPAComponent = &componentCopy
	}
	if installed != nil {
		installedCopy := *installed
		transition.Installed = &installedCopy
	}
	s.state.SlimTransition = &transition
	if err := s.persistMutationLocked(previous); err != nil {
		return SlimTransition{}, err
	}
	return *cloneSlimTransition(s.state.SlimTransition), nil
}

func (s *StateStore) ClearSlimTransition() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.SlimTransition == nil {
		return nil
	}
	previous := cloneState(s.state)
	s.state.SlimTransition = nil
	return s.persistMutationLocked(previous)
}

func (s *StateStore) RestoreSlimTransition() (model.DeploymentState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.SlimTransition == nil {
		return s.state.Deployment, nil
	}
	previous := cloneState(s.state)
	transition := cloneSlimTransition(s.state.SlimTransition)
	s.state.Deployment = cloneDeploymentState(transition.PreviousDeployment)
	s.state.CPAUpstreamURL = transition.PreviousCPAUpstreamURL
	if transition.PreviousCPAComponent != nil {
		s.state.Components["cpa"] = *transition.PreviousCPAComponent
	} else {
		delete(s.state.Components, "cpa")
	}
	s.state.SlimTransition = nil
	if err := s.persistMutationLocked(previous); err != nil {
		return model.DeploymentState{}, err
	}
	return s.state.Deployment, nil
}

func (s *StateStore) migrate() error {
	if s.state.SchemaVersion == 0 || s.state.SchemaVersion == 1 || s.state.SchemaVersion == 2 {
		s.state.SchemaVersion = runtimeStateSchemaVersion
	}
	if s.state.SchemaVersion != runtimeStateSchemaVersion {
		return fmt.Errorf("unsupported runtime state schema version %d", s.state.SchemaVersion)
	}
	if s.state.Components == nil {
		s.state.Components = map[string]ComponentState{}
	}
	if s.state.Operations == nil {
		s.state.Operations = map[string]UpdateOperation{}
	}
	if s.state.Deployment.SchemaVersion == 0 {
		s.state.Deployment.SchemaVersion = 1
	}
	canonicalDeployment := model.DefaultDeploymentState(
		s.state.Deployment.Mode,
		s.state.Deployment.PanelBasePath,
		s.state.Deployment.PanelBasePathSource,
	)
	s.state.Deployment.RuntimeManaged = canonicalDeployment.RuntimeManaged
	s.state.Deployment.CPAMPUpdatesManaged = canonicalDeployment.CPAMPUpdatesManaged
	s.state.Deployment.CPAUpdatesManaged = canonicalDeployment.CPAUpdatesManaged
	now := time.Now().UnixMilli()
	for id, operation := range s.state.Operations {
		switch operation.Status {
		case runtimeUpdateStatusQueued, runtimeUpdateStatusRunning, runtimeUpdateStatusRollingBack:
			operation = s.recoverInterruptedOperation(operation, now)
			s.state.Operations[id] = operation
		}
	}
	return s.persistLocked()
}

func (s *StateStore) recoverInterruptedOperation(operation UpdateOperation, now int64) UpdateOperation {
	operation.Status = "failed"
	operation.Progress = 100
	operation.Message = "update interrupted by runtime restart"
	operation.Error = "runtime process restarted before the update completed"
	operation.UpdatedAtMS = now
	operation.CompletedAtMS = now
	if operation.Results == nil {
		operation.Results = map[string]ComponentUpdateResult{}
	}

	rollbackAttempted := false
	rollbackSuccessful := true
	rollbackErrors := make([]string, 0)
	for _, name := range operation.Components {
		previousVersion := strings.TrimSpace(operation.CurrentVersions[name])
		previousBinary := strings.TrimSpace(operation.CurrentBinaries[name])
		if previousVersion == "" && previousBinary == "" {
			continue
		}
		component := s.state.Components[name]
		if component.Version == previousVersion && component.BinaryPath == previousBinary {
			continue
		}

		rollbackAttempted = true
		if previousBinary == "" {
			rollbackSuccessful = false
			rollbackErrors = append(rollbackErrors, name+": previous binary path is unavailable")
			result := operation.Results[name]
			result.Name = name
			result.FromVersion = previousVersion
			result.ToVersion = operation.TargetVersions[name]
			result.Status = "rollback_failed"
			result.Error = strings.TrimSpace(result.Error + "; rollback: previous binary path is unavailable")
			operation.Results[name] = result
			continue
		}
		info, err := os.Stat(previousBinary)
		if err != nil || info.IsDir() {
			rollbackSuccessful = false
			reason := "previous binary is unavailable"
			if err != nil {
				reason = err.Error()
			}
			rollbackErrors = append(rollbackErrors, name+": "+reason)
			result := operation.Results[name]
			result.Name = name
			result.FromVersion = previousVersion
			result.ToVersion = operation.TargetVersions[name]
			result.Status = "rollback_failed"
			result.Error = strings.TrimSpace(result.Error + "; rollback: " + reason)
			operation.Results[name] = result
			continue
		}

		component.Name = name
		component.Version = previousVersion
		component.BinaryPath = previousBinary
		component.Status = "recovered"
		component.PID = 0
		component.LastError = ""
		component.UpdatedAtMS = now
		s.state.Components[name] = component
		result := operation.Results[name]
		result.Name = name
		result.FromVersion = previousVersion
		result.ToVersion = operation.TargetVersions[name]
		result.Status = "rolled_back"
		result.RollbackVersion = previousVersion
		operation.Results[name] = result
	}

	operation.RollbackAttempted = rollbackAttempted
	operation.RollbackSuccessful = rollbackAttempted && rollbackSuccessful
	if rollbackAttempted && rollbackSuccessful {
		operation.Status = "rolled_back"
		operation.Message = "interrupted update rolled back during runtime startup"
	}
	if len(rollbackErrors) > 0 {
		operation.Error += "; rollback failed: " + strings.Join(rollbackErrors, "; ")
	}
	return operation
}

func (s *StateStore) persistMutationLocked(previous State) error {
	if err := s.persistLocked(); err != nil {
		s.state = previous
		return err
	}
	return nil
}

func (s *StateStore) persistLocked() error {
	s.state.UpdatedAtMS = time.Now().UnixMilli()
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".state-*.json")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return replaceRuntimeFile(temporaryPath, s.path)
}

func cloneState(state State) State {
	cloned := state
	cloned.Deployment = cloneDeploymentState(state.Deployment)
	cloned.Components = make(map[string]ComponentState, len(state.Components))
	for key, value := range state.Components {
		cloned.Components[key] = value
	}
	cloned.SlimTransition = cloneSlimTransition(state.SlimTransition)
	cloned.Operations = make(map[string]UpdateOperation, len(state.Operations))
	for key, value := range state.Operations {
		cloned.Operations[key] = cloneOperation(value)
	}
	return cloned
}

func cloneDeploymentState(state model.DeploymentState) model.DeploymentState {
	cloned := state
	cloned.MigrationCheckpoints = append([]string(nil), state.MigrationCheckpoints...)
	cloned.RetiredPanelPaths = append([]string(nil), state.RetiredPanelPaths...)
	return cloned
}

func cloneSlimTransition(transition *SlimTransition) *SlimTransition {
	if transition == nil {
		return nil
	}
	cloned := *transition
	cloned.PreviousDeployment = cloneDeploymentState(transition.PreviousDeployment)
	if transition.PreviousCPAComponent != nil {
		component := *transition.PreviousCPAComponent
		cloned.PreviousCPAComponent = &component
	}
	if transition.Installed != nil {
		installed := *transition.Installed
		cloned.Installed = &installed
	}
	return &cloned
}

func cloneOperation(operation UpdateOperation) UpdateOperation {
	cloned := operation
	cloned.Components = append([]string(nil), operation.Components...)
	cloned.CurrentVersions = cloneStringMap(operation.CurrentVersions)
	cloned.CurrentBinaries = cloneStringMap(operation.CurrentBinaries)
	cloned.TargetVersions = cloneStringMap(operation.TargetVersions)
	cloned.ReleaseURLs = cloneStringMap(operation.ReleaseURLs)
	cloned.ReleaseNotes = cloneStringMap(operation.ReleaseNotes)
	cloned.Results = make(map[string]ComponentUpdateResult, len(operation.Results))
	for key, value := range operation.Results {
		cloned.Results[key] = value
	}
	return cloned
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func (s *StateStore) pruneOperationsLocked(limit int) {
	if len(s.state.Operations) <= limit {
		return
	}
	type operationAge struct {
		id        string
		updatedAt int64
	}
	ages := make([]operationAge, 0, len(s.state.Operations))
	for id, operation := range s.state.Operations {
		ages = append(ages, operationAge{id: id, updatedAt: operation.UpdatedAtMS})
	}
	slices.SortFunc(ages, func(left operationAge, right operationAge) int {
		switch {
		case left.updatedAt < right.updatedAt:
			return -1
		case left.updatedAt > right.updatedAt:
			return 1
		default:
			return strings.Compare(left.id, right.id)
		}
	})
	for len(ages) > limit {
		delete(s.state.Operations, ages[0].id)
		ages = ages[1:]
	}
}
