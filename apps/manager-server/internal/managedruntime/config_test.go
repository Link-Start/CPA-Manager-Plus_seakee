package managedruntime

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestLoadConfigInfersLegacyExternalDeploymentBeforeEmbeddedCPA(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "usage.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec(`create table settings(key text primary key, value text not null, updated_at_ms integer not null)`); err != nil {
		t.Fatalf("create settings: %v", err)
	}
	if _, err := db.Exec(`insert into settings(key, value, updated_at_ms) values('manager_config_v1', ?, 1)`, `{"cpaConnection":{"cpaBaseUrl":"http://legacy-cpa:8317","managementKey":"encrypted"}}`); err != nil {
		t.Fatalf("insert manager config: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}
	before, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read sqlite before inference: %v", err)
	}
	fakeCPA := filepath.Join(dir, "cli-proxy-api")
	if err := os.WriteFile(fakeCPA, []byte("binary"), 0o755); err != nil {
		t.Fatalf("write fake CPA: %v", err)
	}
	setRuntimeTestEnv(t, dir, dbPath)
	t.Setenv("CPA_MANAGER_RUNTIME_CPA_BINARY", fakeCPA)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.DeploymentMode != "external" || cfg.CPAURL != "http://legacy-cpa:8317" {
		t.Fatalf("legacy deployment = mode %q CPA %q", cfg.DeploymentMode, cfg.CPAURL)
	}
	if cfg.UsageDBPath != dbPath {
		t.Fatalf("usage DB path = %q, want %q", cfg.UsageDBPath, dbPath)
	}
	after, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read sqlite after inference: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("legacy deployment inference modified usage.sqlite")
	}
}

func TestLoadConfigInfersInstallerManagedFromUpstreamEnvironment(t *testing.T) {
	dir := t.TempDir()
	setRuntimeTestEnv(t, dir, filepath.Join(dir, "missing.sqlite"))
	t.Setenv("CPA_UPSTREAM_URL", "http://installer-cpa:8317")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.DeploymentMode != "installer-managed" || cfg.CPAURL != "http://installer-cpa:8317" || !cfg.CPAUpstreamEnvSet {
		t.Fatalf("installer deployment = mode %q CPA %q", cfg.DeploymentMode, cfg.CPAURL)
	}
}

func TestLoadConfigInfersIntegratedWhenEmbeddedCPAIsAvailable(t *testing.T) {
	dir := t.TempDir()
	setRuntimeTestEnv(t, dir, filepath.Join(dir, "missing.sqlite"))
	fakeCPA := filepath.Join(dir, "cli-proxy-api")
	if err := os.WriteFile(fakeCPA, []byte("binary"), 0o755); err != nil {
		t.Fatalf("write fake CPA: %v", err)
	}
	t.Setenv("CPA_MANAGER_RUNTIME_CPA_BINARY", fakeCPA)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.DeploymentMode != "integrated" || cfg.CPAURL != "http://127.0.0.1:8317" {
		t.Fatalf("integrated deployment = mode %q CPA %q", cfg.DeploymentMode, cfg.CPAURL)
	}
}

func TestLoadConfigMarksExplicitPanelBasePathAsEnvironmentManaged(t *testing.T) {
	dir := t.TempDir()
	setRuntimeTestEnv(t, dir, filepath.Join(dir, "missing.sqlite"))
	t.Setenv("CPA_MANAGER_PANEL_BASE_PATH", "/admin")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.PanelBasePath != "/admin" || cfg.PanelBasePathSource != "environment" {
		t.Fatalf("panel Base Path = %q from %q", cfg.PanelBasePath, cfg.PanelBasePathSource)
	}
}

func TestLoadConfigDefaultsToAllCompatibilityGatewayPorts(t *testing.T) {
	dir := t.TempDir()
	setRuntimeTestEnv(t, dir, filepath.Join(dir, "missing.sqlite"))
	t.Setenv("CPA_MANAGER_GATEWAY_ADDRS", "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	want := []string{"0.0.0.0:8137", "0.0.0.0:18137", "0.0.0.0:18317"}
	if !slices.Equal(cfg.GatewayAddrs, want) {
		t.Fatalf("gateway addresses = %v, want %v", cfg.GatewayAddrs, want)
	}
}

func TestFindCPABinaryAcceptsWindowsCompanion(t *testing.T) {
	dir := t.TempDir()
	managerBinary := filepath.Join(dir, "cpa-manager-plus.exe")
	cpaBinary := filepath.Join(dir, "cli-proxy-api.exe")
	if err := os.WriteFile(cpaBinary, []byte("binary"), 0o755); err != nil {
		t.Fatalf("write Windows CPA companion: %v", err)
	}

	if got := findCPABinary(managerBinary); got != cpaBinary {
		t.Fatalf("findCPABinary() = %q, want %q", got, cpaBinary)
	}
}

func setRuntimeTestEnv(t *testing.T, dataDir string, dbPath string) {
	t.Helper()
	t.Setenv("CPA_MANAGER_RUNTIME_DATA_DIR", dataDir)
	t.Setenv("USAGE_DATA_DIR", dataDir)
	t.Setenv("USAGE_DB_PATH", dbPath)
	t.Setenv("CPA_MANAGER_DEPLOYMENT_MODE", "")
	t.Setenv("CPA_UPSTREAM_URL", "")
	t.Setenv("CPA_MANAGER_RUNTIME_CPA_BINARY", "")
	t.Setenv("CPA_MANAGER_PANEL_BASE_PATH", "")
	t.Setenv("CPA_MANAGER_GATEWAY_ADDRS", "127.0.0.1:18137")
	t.Setenv("CPA_MANAGER_RUNTIME_MANAGER_ADDR", "127.0.0.1:18318")
	t.Setenv("CPA_MANAGER_RUNTIME_CPA_ADDR", "127.0.0.1:8317")
	t.Setenv("CPA_MANAGER_RUNTIME_CONTROL_ADDR", "127.0.0.1:18319")
}
