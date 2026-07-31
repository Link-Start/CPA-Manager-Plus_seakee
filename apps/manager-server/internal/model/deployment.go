package model

import (
	"errors"
	"net/url"
	"path"
	"strings"
	"unicode"
)

type DeploymentMode string

const (
	DeploymentModeExternal         DeploymentMode = "external"
	DeploymentModeInstallerManaged DeploymentMode = "installer-managed"
	DeploymentModeIntegrated       DeploymentMode = "integrated"
	DeploymentModeSlim             DeploymentMode = "slim"
)

const (
	PanelBasePathSourceDefault     = "default"
	PanelBasePathSourceEnvironment = "environment"
	PanelBasePathSourceRuntime     = "runtime"
	PanelBasePathSourceDatabase    = "db"
)

const (
	BootstrapStateVersion = 2

	BootstrapStatusFresh      = "fresh"
	BootstrapStatusNeedsSetup = "needs_setup"
	BootstrapStatusNeedsAdmin = "needs_admin"
	BootstrapStatusReady      = "ready"
	BootstrapStatusMigrated   = "migrated"

	SetupStepCPAConnection = "cpa_connection"
	SetupStepAdminKey      = "admin_key"
	SetupStepComplete      = "complete"
)

type DeploymentState struct {
	SchemaVersion        int            `json:"schemaVersion"`
	Mode                 DeploymentMode `json:"mode"`
	PanelBasePath        string         `json:"panelBasePath"`
	PanelBasePathSource  string         `json:"panelBasePathSource"`
	RuntimeManaged       bool           `json:"runtimeManaged"`
	CPAMPUpdatesManaged  bool           `json:"cpampUpdatesManaged"`
	CPAUpdatesManaged    bool           `json:"cpaUpdatesManaged"`
	MigrationVersion     int            `json:"migrationVersion"`
	MigrationCheckpoints []string       `json:"migrationCheckpoints,omitempty"`
	RetiredPanelPaths    []string       `json:"retiredPanelPaths,omitempty"`
	UpdatedAtMS          int64          `json:"updatedAtMs"`
}

func NormalizePanelBasePath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "/management.html", nil
	}
	if strings.ContainsAny(value, "?#\\") || !strings.HasPrefix(value, "/") {
		return "", errors.New("panel base path must be an absolute URL path without query or fragment")
	}
	decoded, err := url.PathUnescape(value)
	if err != nil {
		return "", errors.New("panel base path contains invalid URL escaping")
	}
	if decoded != value {
		return "", errors.New("panel base path must not contain URL escaping")
	}
	if strings.IndexFunc(decoded, unicode.IsControl) >= 0 {
		return "", errors.New("panel base path must not contain control characters")
	}
	if strings.Contains(decoded, "\\") || hasTraversalSegment(decoded) {
		return "", errors.New("panel base path must not contain traversal segments")
	}
	normalized := path.Clean(value)
	if normalized == "." {
		normalized = "/"
	}
	for _, reserved := range []string{
		"/health",
		"/healthz",
		"/status",
		"/keep-alive",
		"/oauth",
		"/oauth-callback",
		"/setup",
		"/usage-service",
		"/v0",
		"/v1",
		"/v1beta",
		"/models",
		"/openai",
		"/backend-api",
		"/anthropic",
		"/codex",
		"/antigravity",
		"/debug",
		"/.well-known",
	} {
		if normalized == reserved || strings.HasPrefix(normalized, reserved+"/") {
			return "", errors.New("panel base path conflicts with a reserved API path")
		}
	}
	return normalized, nil
}

func NormalizeDeploymentMode(value string) DeploymentMode {
	switch DeploymentMode(strings.ToLower(strings.TrimSpace(value))) {
	case DeploymentModeInstallerManaged:
		return DeploymentModeInstallerManaged
	case DeploymentModeIntegrated:
		return DeploymentModeIntegrated
	case DeploymentModeSlim:
		return DeploymentModeSlim
	default:
		return DeploymentModeExternal
	}
}

func DefaultDeploymentState(mode DeploymentMode, panelBasePath string, panelBasePathSource string) DeploymentState {
	mode = NormalizeDeploymentMode(string(mode))
	integrated := mode == DeploymentModeIntegrated
	runtimeManaged := integrated || mode == DeploymentModeSlim || mode == DeploymentModeInstallerManaged
	return DeploymentState{
		SchemaVersion:       1,
		Mode:                mode,
		PanelBasePath:       panelBasePath,
		PanelBasePathSource: panelBasePathSource,
		RuntimeManaged:      runtimeManaged,
		CPAMPUpdatesManaged: integrated,
		CPAUpdatesManaged:   integrated,
	}
}

func PanelBasePathEnvironmentManaged(source string) bool {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "env", PanelBasePathSourceEnvironment:
		return true
	default:
		return false
	}
}

func hasTraversalSegment(value string) bool {
	for _, segment := range strings.Split(value, "/") {
		if segment == "." || segment == ".." {
			return true
		}
	}
	return false
}

func DeriveBootstrapStatus(projectInitialized bool, adminReady bool, historical bool) string {
	if projectInitialized && adminReady {
		if historical {
			return BootstrapStatusMigrated
		}
		return BootstrapStatusReady
	}
	if !adminReady && projectInitialized {
		return BootstrapStatusNeedsAdmin
	}
	if historical {
		return BootstrapStatusNeedsSetup
	}
	return BootstrapStatusFresh
}

func DeriveSetupStep(projectInitialized bool, adminReady bool) string {
	if !projectInitialized {
		return SetupStepCPAConnection
	}
	if !adminReady {
		return SetupStepAdminKey
	}
	return SetupStepComplete
}
