package model

import "testing"

func TestNormalizeDeploymentModePreservesSlim(t *testing.T) {
	got := NormalizeDeploymentMode(" slim ")
	if got != DeploymentModeSlim {
		t.Fatalf("NormalizeDeploymentMode(slim) = %q", got)
	}
	state := DefaultDeploymentState(got, "/panel", "runtime")
	if !state.RuntimeManaged || state.CPAMPUpdatesManaged || state.CPAUpdatesManaged {
		t.Fatalf("slim deployment capabilities = %+v", state)
	}
}

func TestInstallerManagedDeploymentKeepsRuntimeControlWithoutManagedUpdates(t *testing.T) {
	state := DefaultDeploymentState(
		DeploymentModeInstallerManaged,
		"/panel",
		PanelBasePathSourceRuntime,
	)
	if !state.RuntimeManaged || state.CPAMPUpdatesManaged || state.CPAUpdatesManaged {
		t.Fatalf("installer-managed deployment capabilities = %+v", state)
	}
}

func TestNormalizePanelBasePathRejectsTraversal(t *testing.T) {
	for _, value := range []string{"/admin/../panel", "/admin/%2e%2e/panel", "/admin\\panel"} {
		if _, err := NormalizePanelBasePath(value); err == nil {
			t.Fatalf("NormalizePanelBasePath(%q) error = nil", value)
		}
	}
	if got, err := NormalizePanelBasePath("/admin/panel/"); err != nil || got != "/admin/panel" {
		t.Fatalf("NormalizePanelBasePath(valid) = %q, %v", got, err)
	}
}

func TestNormalizePanelBasePathRejectsEscapedAndReservedGatewayPaths(t *testing.T) {
	for _, value := range []string{
		"/pan%65l",
		"/healthz",
		"/keep-alive",
		"/oauth",
		"/oauth/callback",
		"/oauth/provider/callback",
		"/oauth-callback",
		"/oauth-callback/provider",
		"/openai/v1",
		"/backend-api/codex",
		"/v1beta/models",
		"/anthropic/callback",
		"/codex/callback",
		"/antigravity/callback",
		"/admin\x00panel",
	} {
		if _, err := NormalizePanelBasePath(value); err == nil {
			t.Fatalf("NormalizePanelBasePath(%q) error = nil", value)
		}
	}
	for _, value := range []string{"/", "/management.html", "/admin", "/panel/nested"} {
		if got, err := NormalizePanelBasePath(value); err != nil || got != value {
			t.Fatalf("NormalizePanelBasePath(%q) = %q, %v", value, got, err)
		}
	}
}

func TestPanelBasePathEnvironmentManagedAcceptsCanonicalAndLegacySources(t *testing.T) {
	for _, source := range []string{PanelBasePathSourceEnvironment, "env", " ENVIRONMENT "} {
		if !PanelBasePathEnvironmentManaged(source) {
			t.Fatalf("PanelBasePathEnvironmentManaged(%q) = false", source)
		}
	}
	for _, source := range []string{"", PanelBasePathSourceDefault, PanelBasePathSourceRuntime, PanelBasePathSourceDatabase} {
		if PanelBasePathEnvironmentManaged(source) {
			t.Fatalf("PanelBasePathEnvironmentManaged(%q) = true", source)
		}
	}
}
