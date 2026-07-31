package setup

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/collector"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/config"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/model"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/problem"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/security"
	collectorservice "github.com/seakee/cpa-manager-plus/apps/manager-server/internal/service/collector"
	managerconfigservice "github.com/seakee/cpa-manager-plus/apps/manager-server/internal/service/managerconfig"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/store"

	_ "modernc.org/sqlite"
)

func TestInfoResultDoesNotExposePanelBasePath(t *testing.T) {
	payload, err := json.Marshal(InfoResult{
		Service:        "cpa-manager-plus",
		Configured:     true,
		AdminReady:     true,
		DeploymentMode: string(model.DeploymentModeIntegrated),
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "panelBasePath") {
		t.Fatalf("public info exposed panel Base Path: %s", payload)
	}
}

func TestSetupRollsBackAllLocalSettingsWhenBootstrapStateWriteFails(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "usage.sqlite")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	initialState := store.BootstrapState{
		Version:           model.BootstrapStateVersion,
		Status:            model.BootstrapStatusFresh,
		SetupStep:         model.SetupStepCPAConnection,
		BootstrapRequired: true,
		DataKeyReady:      true,
	}
	if err := st.SaveBootstrapState(t.Context(), initialState); err != nil {
		t.Fatalf("save initial bootstrap state: %v", err)
	}
	beforeState, ok, err := st.LoadBootstrapState(t.Context())
	if err != nil || !ok {
		t.Fatalf("load initial bootstrap state: ok=%v err=%v", ok, err)
	}
	installBootstrapStateUpdateFailure(t, dbPath)

	cpaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v0/management/config" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(cpaServer.Close)

	service := newSetupTestService(st, config.Config{
		DBPath:       dbPath,
		Queue:        "usage",
		PopSide:      "right",
		BatchSize:    100,
		PollInterval: 500 * time.Millisecond,
		QueryLimit:   50000,
	})
	monitoring := false
	_, err = service.Setup(t.Context(), Request{
		CPAUpstreamURL:           cpaServer.URL,
		CPAManagementKey:         "management-key",
		RequestMonitoringEnabled: &monitoring,
	}, "")
	if err == nil {
		t.Fatal("expected setup transaction failure")
	}
	if _, ok, loadErr := st.LoadSetup(t.Context()); loadErr != nil || ok {
		t.Fatalf("setup persisted after rollback: ok=%v err=%v", ok, loadErr)
	}
	if _, ok, loadErr := st.LoadManagerConfig(t.Context()); loadErr != nil || ok {
		t.Fatalf("manager config persisted after rollback: ok=%v err=%v", ok, loadErr)
	}
	afterState, ok, loadErr := st.LoadBootstrapState(t.Context())
	if loadErr != nil || !ok {
		t.Fatalf("load bootstrap state after rollback: ok=%v err=%v", ok, loadErr)
	}
	if afterState != beforeState {
		t.Fatalf("bootstrap state changed after rollback: before=%+v after=%+v", beforeState, afterState)
	}
}

func TestInitializeAdminRollsBackCredentialAndBootstrapConsumptionOnStateFailure(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "usage.sqlite")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	bootstrapCredential, err := security.NewBootstrapCredential("bootstrap-token", time.Hour)
	if err != nil {
		t.Fatalf("create bootstrap credential: %v", err)
	}
	if err := st.SaveBootstrapCredential(t.Context(), bootstrapCredential); err != nil {
		t.Fatalf("save bootstrap credential: %v", err)
	}
	initialState := store.BootstrapState{
		Version:           model.BootstrapStateVersion,
		Status:            model.BootstrapStatusFresh,
		SetupStep:         model.SetupStepCPAConnection,
		BootstrapRequired: true,
		DataKeyReady:      true,
	}
	if err := st.SaveBootstrapState(t.Context(), initialState); err != nil {
		t.Fatalf("save initial bootstrap state: %v", err)
	}
	beforeCredential, _, err := st.LoadBootstrapCredential(t.Context())
	if err != nil {
		t.Fatalf("load bootstrap credential: %v", err)
	}
	beforeState, _, err := st.LoadBootstrapState(t.Context())
	if err != nil {
		t.Fatalf("load bootstrap state: %v", err)
	}
	installBootstrapStateUpdateFailure(t, dbPath)

	service := newSetupTestService(st, config.Config{DBPath: dbPath})
	_, err = service.InitializeAdmin(t.Context(), AdminKeyRequest{AdminKey: "cpamp_AdminKey!123456789"})
	if err == nil {
		t.Fatal("expected admin initialization transaction failure")
	}
	if _, ok, loadErr := st.LoadAdminCredential(t.Context()); loadErr != nil || ok {
		t.Fatalf("admin credential persisted after rollback: ok=%v err=%v", ok, loadErr)
	}
	afterCredential, ok, loadErr := st.LoadBootstrapCredential(t.Context())
	if loadErr != nil || !ok {
		t.Fatalf("load bootstrap credential after rollback: ok=%v err=%v", ok, loadErr)
	}
	if afterCredential != beforeCredential {
		t.Fatalf("bootstrap credential changed after rollback: before=%+v after=%+v", beforeCredential, afterCredential)
	}
	afterState, ok, loadErr := st.LoadBootstrapState(t.Context())
	if loadErr != nil || !ok {
		t.Fatalf("load bootstrap state after rollback: ok=%v err=%v", ok, loadErr)
	}
	if afterState != beforeState {
		t.Fatalf("bootstrap state changed after rollback: before=%+v after=%+v", beforeState, afterState)
	}
}

func TestInitializeAdminAllowsOnlyOneConcurrentInitialization(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "usage.sqlite")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	bootstrapCredential, err := security.NewBootstrapCredential("bootstrap-token", time.Hour)
	if err != nil {
		t.Fatalf("create bootstrap credential: %v", err)
	}
	if err := st.SaveBootstrapCredential(t.Context(), bootstrapCredential); err != nil {
		t.Fatalf("save bootstrap credential: %v", err)
	}
	if err := st.SaveBootstrapState(t.Context(), store.BootstrapState{
		Version:           model.BootstrapStateVersion,
		Status:            model.BootstrapStatusFresh,
		SetupStep:         model.SetupStepCPAConnection,
		BootstrapRequired: true,
		DataKeyReady:      true,
	}); err != nil {
		t.Fatalf("save bootstrap state: %v", err)
	}

	service := newSetupTestService(st, config.Config{DBPath: dbPath})
	keys := []string{"cpamp_FirstAdmin!123456789", "cpamp_SecondAdmin!12345678"}
	type attempt struct {
		key string
		err error
	}
	results := make(chan attempt, len(keys))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, key := range keys {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			<-start
			_, err := service.InitializeAdmin(context.Background(), AdminKeyRequest{AdminKey: key})
			results <- attempt{key: key, err: err}
		}(key)
	}
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	winner := ""
	for result := range results {
		if result.err == nil {
			successes++
			winner = result.key
			continue
		}
		setupProblem, ok := problem.As(result.err)
		if !ok || setupProblem.Code != "setup_admin_already_initialized" {
			t.Fatalf("unexpected concurrent initialization error: %v", result.err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful initializations = %d, want 1", successes)
	}
	adminCredential, ok, err := st.LoadAdminCredential(t.Context())
	if err != nil || !ok || !security.VerifyAdminKey(adminCredential, winner) {
		t.Fatalf("stored winning credential: ok=%v err=%v winner=%q", ok, err, winner)
	}
	consumedBootstrap, ok, err := st.LoadBootstrapCredential(t.Context())
	if err != nil || !ok || consumedBootstrap.ConsumedAtMS == 0 {
		t.Fatalf("bootstrap credential was not consumed: ok=%v err=%v credential=%+v", ok, err, consumedBootstrap)
	}
	state, ok, err := st.LoadBootstrapState(t.Context())
	if err != nil || !ok || !state.AdminReady || state.BootstrapRequired {
		t.Fatalf("bootstrap state after initialization: ok=%v err=%v state=%+v", ok, err, state)
	}
}

func TestAuthorizeSetupMapsBoundedAdminVerificationSaturation(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	credential, err := security.NewAdminCredential("cpamp_Admin!0123456789", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveAdminCredential(t.Context(), credential); err != nil {
		t.Fatal(err)
	}
	service := newSetupTestService(st, config.Config{})
	service.verifyAdminKey = func(model.AdminCredential, string) (bool, error) {
		return false, security.ErrAdminKeyVerificationBusy
	}
	err = service.AuthorizeSetup(t.Context(), "Bearer cpamp_Admin!0123456789", "")
	typed, ok := problem.As(err)
	if !ok || typed.Code != "admin_verification_busy" || typed.Status != http.StatusTooManyRequests ||
		!strings.HasSuffix(typed.DocsURL, "#admin-verification-busy") {
		t.Fatalf("setup authorization error = %#v", typed)
	}
}

func newSetupTestService(st *store.Store, cfg config.Config) *Service {
	collectorManager := collector.NewManager(cfg, st)
	collectorService := collectorservice.New(collectorManager)
	managerConfigService := managerconfigservice.New(cfg, st, collectorService)
	return New(cfg, st, collectorService, managerConfigService, time.Now().UnixMilli(), "test-service")
}

func installBootstrapStateUpdateFailure(t testing.TB, dbPath string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw sqlite: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(
		`create trigger fail_selected_setting_update
		 before update on settings
		 when new.key = 'bootstrap_state_v1'
		 begin
		   select raise(abort, 'forced setting update failure');
		 end`,
	); err != nil {
		t.Fatalf("install setting failure trigger: %v", err)
	}
}
