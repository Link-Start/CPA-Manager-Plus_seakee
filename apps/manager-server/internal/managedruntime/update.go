package managedruntime

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/model"
)

type UpdateCheckComponent struct {
	Name                    string `json:"name"`
	CurrentVersion          string `json:"currentVersion,omitempty"`
	AvailableVersion        string `json:"availableVersion,omitempty"`
	UpdateAvailable         bool   `json:"updateAvailable"`
	ReleaseURL              string `json:"releaseUrl,omitempty"`
	ReleaseNotes            string `json:"releaseNotes,omitempty"`
	MinCPAMPVersion         string `json:"minCpampVersion,omitempty"`
	StandaloneUpdateAllowed bool   `json:"standaloneUpdateAllowed"`
	CombinedUpdateAllowed   bool   `json:"combinedUpdateAllowed"`
}

type UpdateCheckResult struct {
	CheckedAtMS int64                           `json:"checkedAtMs"`
	Deployment  model.DeploymentState           `json:"deployment"`
	Components  map[string]UpdateCheckComponent `json:"components"`
}

type StartUpdateResult struct {
	Operation      UpdateOperation `json:"operation"`
	OperationToken string          `json:"operationToken"`
}

type appliedComponent struct {
	name            string
	previousVersion string
	previousBinary  string
	targetVersion   string
}

func (c *RuntimeController) CheckUpdates(ctx context.Context) (UpdateCheckResult, error) {
	snapshot := c.state.Snapshot()
	if snapshot.Deployment.Mode != model.DeploymentModeIntegrated ||
		!snapshot.Deployment.CPAMPUpdatesManaged ||
		!snapshot.Deployment.CPAUpdatesManaged {
		return UpdateCheckResult{}, errors.New("updates are only available for an Integrated deployment")
	}
	manifest, err := c.installer.FetchManifest(ctx)
	if err != nil {
		return UpdateCheckResult{}, err
	}
	result := UpdateCheckResult{
		CheckedAtMS: time.Now().UnixMilli(),
		Deployment:  snapshot.Deployment,
		Components:  map[string]UpdateCheckComponent{},
	}
	for _, name := range []string{"cpamp", "cpa"} {
		available, ok := manifest.Components[name]
		if !ok {
			return UpdateCheckResult{}, fmt.Errorf("release manifest does not contain component %q", name)
		}
		current := c.currentComponentVersion(snapshot, name)
		result.Components[name] = UpdateCheckComponent{
			Name:                    name,
			CurrentVersion:          current,
			AvailableVersion:        available.Version,
			UpdateAvailable:         releaseVersionIsNewer(available.Version, current),
			ReleaseURL:              available.ReleaseURL,
			ReleaseNotes:            available.ReleaseNotes,
			MinCPAMPVersion:         available.MinCPAMPVersion,
			StandaloneUpdateAllowed: true,
			CombinedUpdateAllowed:   true,
		}
	}
	cpa := result.Components["cpa"]
	if minimum := strings.TrimSpace(cpa.MinCPAMPVersion); minimum != "" {
		cpamp := result.Components["cpamp"]
		cpa.StandaloneUpdateAllowed = !releaseVersionIsNewer(minimum, cpamp.CurrentVersion)
		combinedCPAMPVersion := cpamp.CurrentVersion
		if cpamp.UpdateAvailable {
			combinedCPAMPVersion = cpamp.AvailableVersion
		}
		cpa.CombinedUpdateAllowed = !releaseVersionIsNewer(minimum, combinedCPAMPVersion)
		result.Components["cpa"] = cpa
	}
	return result, nil
}

func (c *RuntimeController) StartUpdate(ctx context.Context, kind string) (StartUpdateResult, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	components, err := updateComponents(kind)
	if err != nil {
		return StartUpdateResult{}, err
	}
	c.runtimeMutationMu.Lock()
	defer c.runtimeMutationMu.Unlock()
	c.updateMu.Lock()
	defer c.updateMu.Unlock()
	snapshot := c.state.Snapshot()
	if snapshot.SlimTransition != nil {
		return StartUpdateResult{}, errors.New("a Slim runtime transition is already pending")
	}
	if snapshot.Deployment.Mode != model.DeploymentModeIntegrated ||
		!snapshot.Deployment.CPAMPUpdatesManaged ||
		!snapshot.Deployment.CPAUpdatesManaged {
		return StartUpdateResult{}, errors.New("updates are only available for an Integrated deployment")
	}
	if c.activeUpdateID != "" {
		if active, ok := c.state.Operation(c.activeUpdateID); ok && runtimeUpdateInProgress(active.Status) {
			return StartUpdateResult{}, fmt.Errorf("runtime update %s is already in progress", c.activeUpdateID)
		}
		c.activeUpdateID = ""
	}
	if active, ok := latestInProgressRuntimeUpdate(c.state.Snapshot()); ok {
		c.activeUpdateID = active.ID
		return StartUpdateResult{}, fmt.Errorf("runtime update %s is already in progress", active.ID)
	}

	manifest, err := c.installer.FetchManifest(ctx)
	if err != nil {
		return StartUpdateResult{}, err
	}
	currentVersions := map[string]string{}
	currentBinaries := map[string]string{}
	targetVersions := map[string]string{}
	releaseURLs := map[string]string{}
	releaseNotes := map[string]string{}
	selected := make(map[string]ReleaseComponent, len(components))
	filtered := make([]string, 0, len(components))
	for _, name := range components {
		component, ok := manifest.Components[name]
		if !ok {
			return StartUpdateResult{}, fmt.Errorf("release manifest does not contain component %q", name)
		}
		current := c.effectiveComponent(snapshot, name)
		if !releaseVersionIsNewer(component.Version, current.Version) {
			continue
		}
		currentVersions[name] = current.Version
		currentBinaries[name] = current.BinaryPath
		targetVersions[name] = component.Version
		releaseURLs[name] = component.ReleaseURL
		releaseNotes[name] = component.ReleaseNotes
		selected[name] = component
		filtered = append(filtered, name)
	}
	if len(filtered) == 0 {
		return StartUpdateResult{}, errors.New("all selected components are already current")
	}
	if cpaRelease, ok := selected["cpa"]; ok && strings.TrimSpace(cpaRelease.MinCPAMPVersion) != "" {
		cpampVersion := c.currentComponentVersion(snapshot, "cpamp")
		if target, updatingCPAMP := targetVersions["cpamp"]; updatingCPAMP {
			cpampVersion = target
		}
		if releaseVersionIsNewer(cpaRelease.MinCPAMPVersion, cpampVersion) {
			return StartUpdateResult{}, fmt.Errorf(
				"CPA %s requires CPAMP %s or newer",
				cpaRelease.Version,
				cpaRelease.MinCPAMPVersion,
			)
		}
	}

	id, token, err := newOperationCredentials()
	if err != nil {
		return StartUpdateResult{}, err
	}
	operation := UpdateOperation{
		ID:                 id,
		Kind:               kind,
		Status:             runtimeUpdateStatusQueued,
		Progress:           0,
		Message:            "update queued",
		Components:         filtered,
		CurrentVersions:    currentVersions,
		CurrentBinaries:    currentBinaries,
		TargetVersions:     targetVersions,
		ReleaseURLs:        releaseURLs,
		ReleaseNotes:       releaseNotes,
		Results:            map[string]ComponentUpdateResult{},
		OperationTokenHash: hashOperationToken(token),
	}
	if err := c.state.SaveOperation(operation); err != nil {
		return StartUpdateResult{}, err
	}
	c.activeUpdateID = id
	go c.runUpdateOperation(id, filtered, selected)
	saved, _ := c.state.Operation(id)
	return StartUpdateResult{Operation: saved, OperationToken: token}, nil
}

func (c *RuntimeController) Operation(id string) (UpdateOperation, bool) {
	return c.state.Operation(strings.TrimSpace(id))
}

func (c *RuntimeController) AuthorizeOperation(id string, token string) bool {
	operation, ok := c.state.Operation(strings.TrimSpace(id))
	if !ok || operation.OperationTokenHash == "" || strings.TrimSpace(token) == "" {
		return false
	}
	expected, err := hex.DecodeString(operation.OperationTokenHash)
	if err != nil {
		return false
	}
	actualSum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return len(expected) == len(actualSum) && subtle.ConstantTimeCompare(expected, actualSum[:]) == 1
}

func (c *RuntimeController) runUpdateOperation(id string, components []string, releases map[string]ReleaseComponent) {
	defer c.clearActiveUpdate(id)
	operationCtx := c.ctx
	if operationCtx == nil {
		operationCtx = context.Background()
	}
	operation, ok := c.state.Operation(id)
	if !ok {
		log.Printf("runtime update %s disappeared before execution", id)
		return
	}
	results := cloneComponentUpdateResults(operation.Results)
	if err := c.persistOperation(id, func(operation UpdateOperation) UpdateOperation {
		operation.Status = runtimeUpdateStatusRunning
		operation.Progress = 5
		operation.Message = "verifying and installing release assets"
		return operation
	}); err != nil {
		log.Printf("start runtime update %s: %v", id, err)
		if finalizeErr := c.persistOperation(id, func(operation UpdateOperation) UpdateOperation {
			operation.Status = runtimeUpdateStatusFailed
			operation.Progress = 100
			operation.Message = "update failed"
			operation.Error = fmt.Errorf("persist running update state: %w", err).Error()
			operation.CompletedAtMS = time.Now().UnixMilli()
			return operation
		}); finalizeErr != nil {
			log.Printf("finalize unstarted runtime update %s: %v", id, finalizeErr)
		}
		return
	}
	applied := make([]appliedComponent, 0, len(components))
	var updateErr error
	for index, name := range components {
		if err := c.persistOperation(id, func(operation UpdateOperation) UpdateOperation {
			operation.Progress = 10 + index*70/len(components)
			operation.Message = "updating " + name
			return operation
		}); err != nil {
			updateErr = fmt.Errorf("persist %s update progress: %w", name, err)
			break
		}
		changed, err := c.applyComponentUpdate(operationCtx, name, releases[name])
		if changed.name != "" {
			applied = append(applied, changed)
		}
		if err != nil {
			updateErr = fmt.Errorf("update %s: %w", name, err)
			results[name] = ComponentUpdateResult{
				Name:        name,
				FromVersion: changed.previousVersion,
				ToVersion:   changed.targetVersion,
				Status:      "failed",
				Error:       err.Error(),
			}
			break
		}
		results[name] = ComponentUpdateResult{
			Name:        name,
			FromVersion: changed.previousVersion,
			ToVersion:   changed.targetVersion,
			Status:      "succeeded",
		}
		if err := c.persistOperation(id, func(operation UpdateOperation) UpdateOperation {
			operation.Results = cloneComponentUpdateResults(results)
			return operation
		}); err != nil {
			updateErr = fmt.Errorf("persist %s update result: %w", name, err)
			break
		}
	}

	requiresHandoff := containsComponent(components, "cpamp")
	if updateErr == nil && requiresHandoff {
		if err := c.persistOperation(id, func(operation UpdateOperation) UpdateOperation {
			operation.Status = runtimeUpdateStatusHandoffPending
			operation.Progress = 95
			operation.Message = "restarting runtime to complete update"
			operation.Results = cloneComponentUpdateResults(results)
			operation.CompletedAtMS = 0
			return operation
		}); err != nil {
			updateErr = fmt.Errorf("persist runtime handoff state: %w", err)
		} else if err := c.requestRuntimeHandoff(operationCtx, id); err != nil {
			updateErr = err
		}
	}
	if updateErr == nil && !requiresHandoff {
		if err := c.persistOperation(id, func(operation UpdateOperation) UpdateOperation {
			operation.Status = runtimeUpdateStatusSucceeded
			operation.Progress = 100
			operation.Message = "update completed"
			operation.Results = cloneComponentUpdateResults(results)
			operation.CompletedAtMS = time.Now().UnixMilli()
			return operation
		}); err != nil {
			updateErr = fmt.Errorf("persist completed runtime update: %w", err)
		}
	}

	if updateErr != nil {
		rollbackOK := true
		operationErr := updateErr
		if err := c.persistOperation(id, func(operation UpdateOperation) UpdateOperation {
			operation.Status = runtimeUpdateStatusRollingBack
			operation.Progress = 90
			operation.Message = "update failed; rolling back"
			operation.Error = updateErr.Error()
			operation.RollbackAttempted = len(applied) > 0
			operation.Results = cloneComponentUpdateResults(results)
			return operation
		}); err != nil {
			operationErr = errors.Join(operationErr, fmt.Errorf("persist rollback state: %w", err))
		}
		for index := len(applied) - 1; index >= 0; index-- {
			changed := applied[index]
			if err := c.rollbackComponent(operationCtx, changed); err != nil {
				rollbackOK = false
				result := results[changed.name]
				result.Name = changed.name
				result.FromVersion = changed.previousVersion
				result.ToVersion = changed.targetVersion
				result.Status = "rollback_failed"
				result.Error = strings.TrimSpace(result.Error + "; rollback: " + err.Error())
				results[changed.name] = result
				operationErr = errors.Join(operationErr, fmt.Errorf("rollback %s: %w", changed.name, err))
				continue
			}
			result := results[changed.name]
			result.Name = changed.name
			result.FromVersion = changed.previousVersion
			result.ToVersion = changed.targetVersion
			result.Status = "rolled_back"
			result.RollbackVersion = changed.previousVersion
			results[changed.name] = result
		}
		if err := c.persistOperation(id, func(operation UpdateOperation) UpdateOperation {
			operation.Status = runtimeUpdateStatusFailed
			if len(applied) > 0 && rollbackOK {
				operation.Status = runtimeUpdateStatusRolledBack
			}
			operation.Progress = 100
			operation.Message = "update failed"
			operation.Error = operationErr.Error()
			operation.RollbackAttempted = len(applied) > 0
			operation.RollbackSuccessful = len(applied) > 0 && rollbackOK
			operation.Results = cloneComponentUpdateResults(results)
			operation.CompletedAtMS = time.Now().UnixMilli()
			return operation
		}); err != nil {
			log.Printf("finalize failed runtime update %s: %v", id, err)
		}
		return
	}
}

func (c *RuntimeController) requestRuntimeHandoff(ctx context.Context, operationID string) error {
	c.mu.Lock()
	handoff := c.handoff
	c.mu.Unlock()
	if handoff == nil {
		return errors.New("runtime handoff channel is unavailable")
	}
	binary := strings.TrimSpace(c.state.Snapshot().Components["cpamp"].BinaryPath)
	if binary == "" {
		return errors.New("updated CPAMP runtime binary is unavailable")
	}
	select {
	case handoff <- runtimeHandoff{Binary: binary, OperationID: operationID}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("request runtime handoff: %w", ctx.Err())
	}
}

func (c *RuntimeController) persistOperation(id string, update func(UpdateOperation) UpdateOperation) error {
	_, err := c.state.UpdateOperation(id, update)
	return err
}

func (c *RuntimeController) clearActiveUpdate(id string) {
	c.updateMu.Lock()
	if c.activeUpdateID == id {
		c.activeUpdateID = ""
	}
	c.updateMu.Unlock()
}

func cloneComponentUpdateResults(results map[string]ComponentUpdateResult) map[string]ComponentUpdateResult {
	cloned := make(map[string]ComponentUpdateResult, len(results))
	for name, result := range results {
		cloned[name] = result
	}
	return cloned
}

func containsComponent(components []string, target string) bool {
	for _, component := range components {
		if component == target {
			return true
		}
	}
	return false
}

func latestInProgressRuntimeUpdate(snapshot State) (UpdateOperation, bool) {
	var latest UpdateOperation
	for _, operation := range snapshot.Operations {
		if !runtimeUpdateInProgress(operation.Status) {
			continue
		}
		if latest.ID == "" || operation.UpdatedAtMS > latest.UpdatedAtMS ||
			(operation.UpdatedAtMS == latest.UpdatedAtMS && operation.ID > latest.ID) {
			latest = operation
		}
	}
	return latest, latest.ID != ""
}

func rejectInProgressRuntimeUpdate(snapshot State) error {
	if active, ok := latestInProgressRuntimeUpdate(snapshot); ok {
		return fmt.Errorf("runtime update %s is already in progress", active.ID)
	}
	return nil
}

func (c *RuntimeController) applyComponentUpdate(ctx context.Context, name string, release ReleaseComponent) (appliedComponent, error) {
	snapshot := c.state.Snapshot()
	previous := c.effectiveComponent(snapshot, name)
	installed, err := c.installer.InstallComponent(ctx, name, release)
	if err != nil {
		return appliedComponent{}, err
	}
	changed := appliedComponent{
		name:            name,
		previousVersion: previous.Version,
		previousBinary:  previous.BinaryPath,
		targetVersion:   installed.Version,
	}
	previousPID := previous.PID
	if err := c.state.UpdateComponent(name, func(component ComponentState) ComponentState {
		component.BinaryPath = installed.BinaryPath
		component.Version = installed.Version
		component.Status = "updating"
		component.LastError = ""
		return component
	}); err != nil {
		return changed, err
	}
	if err := c.supervisor.Restart(name); err != nil {
		return changed, err
	}
	if err := c.waitComponentRestart(ctx, name, installed.Version, previousPID, 90*time.Second); err != nil {
		return changed, err
	}
	cfg := c.configSnapshot()
	if err := syncCurrentComponentPointer(cfg.ComponentDir, name, installed.Version, installed.BinaryPath); err != nil {
		return changed, fmt.Errorf("sync current %s component: %w", name, err)
	}
	return changed, nil
}

func (c *RuntimeController) rollbackComponent(ctx context.Context, changed appliedComponent) error {
	snapshot := c.state.Snapshot()
	previousPID := snapshot.Components[changed.name].PID
	if err := c.state.UpdateComponent(changed.name, func(component ComponentState) ComponentState {
		component.BinaryPath = changed.previousBinary
		component.Version = changed.previousVersion
		component.Status = "rolling_back"
		return component
	}); err != nil {
		return err
	}
	if err := c.supervisor.Restart(changed.name); err != nil {
		return err
	}
	if err := c.waitComponentRestart(ctx, changed.name, changed.previousVersion, previousPID, 90*time.Second); err != nil {
		return err
	}
	cfg := c.configSnapshot()
	if err := syncCurrentComponentPointer(cfg.ComponentDir, changed.name, changed.previousVersion, changed.previousBinary); err != nil {
		return fmt.Errorf("sync rolled back %s component: %w", changed.name, err)
	}
	return nil
}

func (c *RuntimeController) waitComponentRestart(ctx context.Context, name string, version string, previousPID int, timeout time.Duration) error {
	cfg := c.configSnapshot()
	deadlineCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		snapshot := c.state.Snapshot()
		component := snapshot.Components[name]
		pidChanged := previousPID == 0 || component.PID != previousPID
		versionReady := strings.TrimSpace(version) == "" || component.Version == version
		if component.Status == "running" && component.PID > 0 && pidChanged && versionReady {
			healthURL := strings.TrimRight(cfg.ManagerURL, "/") + "/health"
			if name == "cpa" {
				healthURL = strings.TrimRight(cfg.CPAURL, "/") + "/healthz"
			}
			if healthyOnce(deadlineCtx, healthURL) {
				return nil
			}
		}
		select {
		case <-deadlineCtx.Done():
			return fmt.Errorf("%s did not become healthy after restart: %w", name, deadlineCtx.Err())
		case <-ticker.C:
		}
	}
}

func (c *RuntimeController) waitRuntimeHandoffReady(
	ctx context.Context,
	operation UpdateOperation,
	timeout time.Duration,
) error {
	if operation.Status != runtimeUpdateStatusHandoffPending {
		return fmt.Errorf("runtime update %s is not awaiting handoff confirmation", operation.ID)
	}
	deadlineCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	snapshot := c.state.Snapshot()
	components := []string{"cpamp"}
	if snapshot.Deployment.Mode == model.DeploymentModeIntegrated {
		components = append(components, "cpa")
	}
	for _, name := range components {
		component := snapshot.Components[name]
		if strings.TrimSpace(component.BinaryPath) == "" {
			return fmt.Errorf("%s runtime binary is unavailable after handoff", name)
		}
		if err := c.waitComponentRestart(
			deadlineCtx,
			name,
			component.Version,
			0,
			timeout,
		); err != nil {
			return fmt.Errorf("confirm runtime handoff for %s: %w", name, err)
		}
	}
	return nil
}

func healthyOnce(ctx context.Context, target string) bool {
	requestCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, target, nil)
	if err != nil {
		return false
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	_ = response.Body.Close()
	return response.StatusCode >= 200 && response.StatusCode < 300
}

func (c *RuntimeController) effectiveComponent(snapshot State, name string) ComponentState {
	cfg := c.configSnapshot()
	component := snapshot.Components[name]
	switch name {
	case "cpamp":
		if component.BinaryPath == "" {
			component.BinaryPath = cfg.ManagerBinary
		}
		if component.Version == "" {
			component.Version = cfg.CPAMPVersion
		}
	case "cpa":
		if component.BinaryPath == "" {
			component.BinaryPath = cfg.CPABinary
		}
		if component.Version == "" {
			component.Version = cfg.CPAVersion
		}
	}
	return component
}

func (c *RuntimeController) currentComponentVersion(snapshot State, name string) string {
	return c.effectiveComponent(snapshot, name).Version
}

func updateComponents(kind string) ([]string, error) {
	switch kind {
	case "cpamp":
		return []string{"cpamp"}, nil
	case "cpa":
		return []string{"cpa"}, nil
	case "all":
		return []string{"cpamp", "cpa"}, nil
	default:
		return nil, errors.New("update target must be cpamp, cpa, or all")
	}
}

func newOperationCredentials() (string, string, error) {
	idBytes := make([]byte, 16)
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(idBytes); err != nil {
		return "", "", err
	}
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", err
	}
	return "update_" + hex.EncodeToString(idBytes), "op_" + hex.EncodeToString(tokenBytes), nil
}

func hashOperationToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func releaseVersionIsNewer(latest string, current string) bool {
	latest = strings.TrimSpace(latest)
	current = strings.TrimSpace(current)
	if latest == "" {
		return false
	}
	if current == "" || strings.EqualFold(current, "dev") || strings.EqualFold(current, "unknown") {
		return true
	}
	comparison, ok := compareReleaseVersions(latest, current)
	if !ok {
		return latest != current
	}
	return comparison > 0
}

func compareReleaseVersions(left string, right string) (int, bool) {
	leftCore, leftPre, leftOK := parseReleaseVersion(left)
	rightCore, rightPre, rightOK := parseReleaseVersion(right)
	if !leftOK || !rightOK {
		return 0, false
	}
	maxLen := len(leftCore)
	if len(rightCore) > maxLen {
		maxLen = len(rightCore)
	}
	for index := 0; index < maxLen; index++ {
		leftPart := 0
		rightPart := 0
		if index < len(leftCore) {
			leftPart = leftCore[index]
		}
		if index < len(rightCore) {
			rightPart = rightCore[index]
		}
		if leftPart < rightPart {
			return -1, true
		}
		if leftPart > rightPart {
			return 1, true
		}
	}
	if len(leftPre) == 0 && len(rightPre) == 0 {
		return 0, true
	}
	if len(leftPre) == 0 {
		return 1, true
	}
	if len(rightPre) == 0 {
		return -1, true
	}
	maxPreLen := len(leftPre)
	if len(rightPre) > maxPreLen {
		maxPreLen = len(rightPre)
	}
	for index := 0; index < maxPreLen; index++ {
		if index >= len(leftPre) {
			return -1, true
		}
		if index >= len(rightPre) {
			return 1, true
		}
		comparison := comparePrereleaseIdentifier(leftPre[index], rightPre[index])
		if comparison != 0 {
			return comparison, true
		}
	}
	return 0, true
}

func parseReleaseVersion(value string) ([]int, []string, bool) {
	value = strings.TrimSpace(strings.TrimLeft(value, "vV"))
	value, _, _ = strings.Cut(value, "+")
	core, prerelease, _ := strings.Cut(value, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return nil, nil, false
	}
	result := make([]int, 0, len(parts))
	for _, part := range parts {
		if len(part) > 1 && strings.HasPrefix(part, "0") {
			return nil, nil, false
		}
		parsed, err := strconv.Atoi(part)
		if err != nil || parsed < 0 {
			return nil, nil, false
		}
		result = append(result, parsed)
	}
	var prereleaseParts []string
	if prerelease != "" {
		prereleaseParts = strings.Split(prerelease, ".")
		for _, identifier := range prereleaseParts {
			if identifier == "" || !isSemVerIdentifier(identifier) {
				return nil, nil, false
			}
			if isNumericIdentifier(identifier) && len(identifier) > 1 && strings.HasPrefix(identifier, "0") {
				return nil, nil, false
			}
		}
	}
	return result, prereleaseParts, true
}

func comparePrereleaseIdentifier(left string, right string) int {
	if left == right {
		return 0
	}
	leftNumeric := isNumericIdentifier(left)
	rightNumeric := isNumericIdentifier(right)
	if leftNumeric && rightNumeric {
		if len(left) < len(right) {
			return -1
		}
		if len(left) > len(right) {
			return 1
		}
		if left < right {
			return -1
		}
		return 1
	}
	if leftNumeric {
		return -1
	}
	if rightNumeric {
		return 1
	}
	if left < right {
		return -1
	}
	return 1
}

func isNumericIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func isSemVerIdentifier(value string) bool {
	for _, char := range value {
		if (char >= '0' && char <= '9') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= 'a' && char <= 'z') ||
			char == '-' {
			continue
		}
		return false
	}
	return true
}
