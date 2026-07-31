package bootstrap

import (
	"context"
	"database/sql"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/config"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/model"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/security"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/store"

	_ "modernc.org/sqlite"
)

func TestRunMigratesLegacySetupAndEncryptsSecrets(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "usage.sqlite")
	legacyStore, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open legacy store: %v", err)
	}
	if err := legacyStore.SaveSetup(context.Background(), store.Setup{
		CPAUpstreamURL: "http://cpa.local:8317",
		ManagementKey:  "management-key",
		Queue:          "usage",
		PopSide:        "right",
	}); err != nil {
		t.Fatalf("save legacy setup: %v", err)
	}
	if err := legacyStore.Close(); err != nil {
		t.Fatalf("close legacy store: %v", err)
	}

	protector, err := security.NewProtector([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("create protector: %v", err)
	}
	st, err := store.Open(dbPath, protector)
	if err != nil {
		t.Fatalf("open protected store: %v", err)
	}
	t.Cleanup(func() {
		_ = st.Close()
	})

	result, err := Run(context.Background(), config.Config{
		DBPath:        dbPath,
		Queue:         "usage",
		PopSide:       "right",
		BatchSize:     100,
		QueryLimit:    50000,
		CollectorMode: "auto",
	}, st, true)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if result.AdminCreated || result.GeneratedAdminKey != "" || !result.BootstrapTokenCreated || result.GeneratedBootstrapToken == "" {
		t.Fatalf("bootstrap credential result = %#v", result)
	}
	if !result.MigratedLegacy || !result.HasHistoricalData || !result.State.ProjectInitialized || result.State.AdminReady {
		t.Fatalf("bootstrap result = %#v", result)
	}

	if _, ok, err := st.LoadAdminCredential(context.Background()); err != nil || ok {
		t.Fatalf("unexpected admin credential ok=%v err=%v", ok, err)
	}
	bootstrapCredential, ok, err := st.LoadBootstrapCredential(context.Background())
	if err != nil || !ok {
		t.Fatalf("load bootstrap credential ok=%v err=%v", ok, err)
	}
	if !security.VerifyBootstrapToken(bootstrapCredential, result.GeneratedBootstrapToken, time.Now()) {
		t.Fatal("generated bootstrap token does not verify")
	}
	if result.State.Status != "needs_admin" || result.State.SetupStep != "admin_key" || !result.State.BootstrapRequired {
		t.Fatalf("bootstrap state = %#v", result.State)
	}

	managerCfg, ok, err := st.LoadManagerConfig(context.Background())
	if err != nil || !ok {
		t.Fatalf("load migrated manager config ok=%v err=%v", ok, err)
	}
	if managerCfg.CPAConnection.CPABaseURL != "http://cpa.local:8317" ||
		managerCfg.CPAConnection.ManagementKey != "management-key" {
		t.Fatalf("migrated manager config = %#v", managerCfg)
	}

	for _, key := range []string{"setup", "manager_config_v1"} {
		raw := rawBootstrapSettingValue(t, dbPath, key)
		if strings.Contains(raw, "management-key") || !strings.Contains(raw, "enc:v1:") {
			t.Fatalf("%s setting was not encrypted: %s", key, raw)
		}
	}
}

func TestRunPreservesExplicitAdminKeyAndDoesNotIssueBootstrapToken(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	const adminKey = "cpamp_ExplicitAdmin123_key"
	result, err := Run(context.Background(), config.Config{
		AdminKey:          adminKey,
		BootstrapTokenTTL: time.Hour,
		PanelBasePath:     "/management.html",
	}, st, false)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if !result.AdminCreated || result.GeneratedBootstrapToken != "" || !result.State.AdminReady {
		t.Fatalf("bootstrap result = %#v", result)
	}
	credential, ok, err := st.LoadAdminCredential(context.Background())
	if err != nil || !ok || !security.VerifyAdminKey(credential, adminKey) {
		t.Fatalf("admin credential ok=%v err=%v credential=%#v", ok, err, credential)
	}
}

func TestEnsureDeploymentStateReconcilesRuntimeManagedConfig(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	existing := model.DefaultDeploymentState(
		model.DeploymentModeIntegrated,
		"/stale-panel",
		model.PanelBasePathSourceDatabase,
	)
	existing.MigrationVersion = 7
	existing.MigrationCheckpoints = []string{"legacy-docker", "runtime-v1"}
	existing.RetiredPanelPaths = []string{"/management.html", "/older-panel"}
	if err := st.SaveDeploymentState(t.Context(), existing); err != nil {
		t.Fatal(err)
	}

	deployment, err := ensureDeploymentState(t.Context(), config.Config{
		DeploymentMode:      string(model.DeploymentModeSlim),
		PanelBasePath:       "/runtime-panel/",
		PanelBasePathSource: model.PanelBasePathSourceRuntime,
	}, st)
	if err != nil {
		t.Fatal(err)
	}
	if deployment.Mode != model.DeploymentModeSlim || deployment.PanelBasePath != "/runtime-panel" ||
		deployment.PanelBasePathSource != model.PanelBasePathSourceRuntime || !deployment.RuntimeManaged ||
		deployment.CPAMPUpdatesManaged || deployment.CPAUpdatesManaged {
		t.Fatalf("reconciled deployment = %+v", deployment)
	}
	if deployment.MigrationVersion != existing.MigrationVersion ||
		strings.Join(deployment.MigrationCheckpoints, ",") != strings.Join(existing.MigrationCheckpoints, ",") ||
		strings.Join(deployment.RetiredPanelPaths, ",") != strings.Join(existing.RetiredPanelPaths, ",") {
		t.Fatalf("reconciled deployment lost migration metadata: %+v", deployment)
	}
	persisted, ok, err := st.LoadDeploymentState(t.Context())
	if err != nil || !ok || persisted.Mode != model.DeploymentModeSlim || persisted.PanelBasePath != "/runtime-panel" {
		t.Fatalf("persisted deployment = %+v, ok=%v err=%v", persisted, ok, err)
	}
}

func TestEnsureDeploymentStateAppliesAndReleasesEnvironmentPanelBasePath(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	existing := model.DefaultDeploymentState(
		model.DeploymentModeExternal,
		"/database-panel",
		model.PanelBasePathSourceDatabase,
	)
	if err := st.SaveDeploymentState(t.Context(), existing); err != nil {
		t.Fatal(err)
	}

	deployment, err := ensureDeploymentState(t.Context(), config.Config{
		DeploymentMode:      string(model.DeploymentModeIntegrated),
		PanelBasePath:       "/environment-panel",
		PanelBasePathSource: model.PanelBasePathSourceEnvironment,
		PanelBasePathEnvSet: true,
	}, st)
	if err != nil {
		t.Fatal(err)
	}
	if deployment.Mode != existing.Mode || deployment.PanelBasePath != "/environment-panel" ||
		deployment.PanelBasePathSource != model.PanelBasePathSourceEnvironment ||
		!slices.Contains(deployment.RetiredPanelPaths, "/database-panel") {
		t.Fatalf("environment panel Base Path was not reconciled: %+v", deployment)
	}

	deployment, err = ensureDeploymentState(t.Context(), config.Config{
		DeploymentMode:      string(model.DeploymentModeExternal),
		PanelBasePath:       "/management.html",
		PanelBasePathSource: model.PanelBasePathSourceDefault,
	}, st)
	if err != nil {
		t.Fatal(err)
	}
	if deployment.PanelBasePath != "/environment-panel" ||
		deployment.PanelBasePathSource != model.PanelBasePathSourceDatabase {
		t.Fatalf("environment panel Base Path was not released: %+v", deployment)
	}
}

func rawBootstrapSettingValue(t testing.TB, dbPath string, key string) string {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw sqlite: %v", err)
	}
	defer db.Close()

	var raw string
	if err := db.QueryRow(`select value from settings where key = ?`, key).Scan(&raw); err != nil {
		t.Fatalf("load raw setting %s: %v", key, err)
	}
	return raw
}
