package setting

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/model"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/security"
)

const managerConfigKey = "manager_config_v1"
const automationSettingsKey = "automation_settings_v1"
const adminCredentialKey = "admin_credential_v1"
const bootstrapStateKey = "bootstrap_state_v1"
const bootstrapCredentialKey = "bootstrap_credential_v1"
const deploymentStateKey = "deployment_state_v1"

type Repository interface {
	SaveManagerConfig(ctx context.Context, cfg model.ManagerConfig) error
	LoadManagerConfig(ctx context.Context) (model.ManagerConfig, bool, error)
	SaveAutomationSettings(ctx context.Context, settings model.AutomationSettings) (model.AutomationSettings, error)
	LoadAutomationSettings(ctx context.Context) (model.AutomationSettings, bool, error)
	SaveSetup(ctx context.Context, setup model.Setup) error
	LoadSetup(ctx context.Context) (model.Setup, bool, error)
	SaveAdminCredential(ctx context.Context, credential model.AdminCredential) error
	LoadAdminCredential(ctx context.Context) (model.AdminCredential, bool, error)
	SaveBootstrapState(ctx context.Context, state model.BootstrapState) error
	LoadBootstrapState(ctx context.Context) (model.BootstrapState, bool, error)
	SaveBootstrapCredential(ctx context.Context, credential model.BootstrapCredential) error
	LoadBootstrapCredential(ctx context.Context) (model.BootstrapCredential, bool, error)
	SaveSetupInitialization(ctx context.Context, setup model.Setup, cfg model.ManagerConfig, state model.BootstrapState) error
	InitializeAdmin(ctx context.Context, credential model.AdminCredential, state model.BootstrapState) (bool, error)
	SaveDeploymentState(ctx context.Context, state model.DeploymentState) error
	LoadDeploymentState(ctx context.Context) (model.DeploymentState, bool, error)
	HasHistoricalData(ctx context.Context) (bool, error)
}

func (r *repository) SaveSetupInitialization(
	ctx context.Context,
	setup model.Setup,
	cfg model.ManagerConfig,
	state model.BootstrapState,
) error {
	if setup.CPAUpstreamURL == "" || setup.ManagementKey == "" {
		return errors.New("cpaBaseUrl and managementKey are required")
	}
	now := time.Now().UnixMilli()
	protectedSetup, err := r.protectSetup(setup)
	if err != nil {
		return err
	}
	cfg.UpdatedAtMS = now
	protectedConfig, err := r.protectManagerConfig(cfg)
	if err != nil {
		return err
	}
	state.UpdatedAtMS = now
	setupData, err := json.Marshal(protectedSetup)
	if err != nil {
		return err
	}
	configData, err := json.Marshal(protectedConfig)
	if err != nil {
		return err
	}
	stateData, err := json.Marshal(state)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, item := range []struct {
		key  string
		data []byte
	}{
		{key: "setup", data: setupData},
		{key: managerConfigKey, data: configData},
		{key: bootstrapStateKey, data: stateData},
	} {
		if _, err := tx.ExecContext(
			ctx,
			`insert into settings(key, value, updated_at_ms)
			 values(?, ?, ?)
			 on conflict(key) do update set value = excluded.value, updated_at_ms = excluded.updated_at_ms`,
			item.key,
			string(item.data),
			now,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *repository) InitializeAdmin(
	ctx context.Context,
	credential model.AdminCredential,
	state model.BootstrapState,
) (bool, error) {
	if credential.KeyHash == "" || credential.Salt == "" {
		return false, errors.New("admin credential is required")
	}
	now := time.Now().UnixMilli()
	state.UpdatedAtMS = now
	credentialData, err := json.Marshal(credential)
	if err != nil {
		return false, err
	}
	stateData, err := json.Marshal(state)
	if err != nil {
		return false, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(
		ctx,
		`insert into settings(key, value, updated_at_ms)
		 values(?, ?, ?)
		 on conflict(key) do nothing`,
		adminCredentialKey,
		string(credentialData),
		now,
	)
	if err != nil {
		return false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rowsAffected == 0 {
		return false, nil
	}

	var bootstrapRaw string
	err = tx.QueryRowContext(ctx, `select value from settings where key = ?`, bootstrapCredentialKey).Scan(&bootstrapRaw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if err == nil {
		var bootstrapCredential model.BootstrapCredential
		if err := json.Unmarshal([]byte(bootstrapRaw), &bootstrapCredential); err != nil {
			return false, err
		}
		if bootstrapCredential.ConsumedAtMS == 0 {
			bootstrapCredential.ConsumedAtMS = now
			bootstrapData, err := json.Marshal(bootstrapCredential)
			if err != nil {
				return false, err
			}
			if _, err := tx.ExecContext(
				ctx,
				`update settings set value = ?, updated_at_ms = ? where key = ?`,
				string(bootstrapData),
				now,
				bootstrapCredentialKey,
			); err != nil {
				return false, err
			}
		}
	}
	if _, err := tx.ExecContext(
		ctx,
		`insert into settings(key, value, updated_at_ms)
		 values(?, ?, ?)
		 on conflict(key) do update set value = excluded.value, updated_at_ms = excluded.updated_at_ms`,
		bootstrapStateKey,
		string(stateData),
		now,
	); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

type repository struct {
	db        *sql.DB
	protector *security.Protector
}

func New(db *sql.DB, protector ...*security.Protector) Repository {
	var p *security.Protector
	if len(protector) > 0 {
		p = protector[0]
	}
	return &repository{db: db, protector: p}
}

func (r *repository) SaveSetup(ctx context.Context, setup model.Setup) error {
	if setup.CPAUpstreamURL == "" || setup.ManagementKey == "" {
		return errors.New("cpaBaseUrl and managementKey are required")
	}
	protected, err := r.protectSetup(setup)
	if err != nil {
		return err
	}
	data, err := json.Marshal(protected)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(
		ctx,
		`insert into settings(key, value, updated_at_ms)
		 values('setup', ?, ?)
		 on conflict(key) do update set value = excluded.value, updated_at_ms = excluded.updated_at_ms`,
		string(data),
		time.Now().UnixMilli(),
	)
	return err
}

func (r *repository) LoadSetup(ctx context.Context) (model.Setup, bool, error) {
	var raw string
	err := r.db.QueryRowContext(ctx, `select value from settings where key = 'setup'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Setup{}, false, nil
	}
	if err != nil {
		return model.Setup{}, false, err
	}
	var setup model.Setup
	if err := json.Unmarshal([]byte(raw), &setup); err != nil {
		return model.Setup{}, false, err
	}
	setup, err = r.unprotectSetup(setup)
	if err != nil {
		return model.Setup{}, false, err
	}
	return setup, true, nil
}

func (r *repository) SaveManagerConfig(ctx context.Context, cfg model.ManagerConfig) error {
	cfg.UpdatedAtMS = time.Now().UnixMilli()
	protected, err := r.protectManagerConfig(cfg)
	if err != nil {
		return err
	}
	data, err := json.Marshal(protected)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(
		ctx,
		`insert into settings(key, value, updated_at_ms)
		 values(?, ?, ?)
		 on conflict(key) do update set value = excluded.value, updated_at_ms = excluded.updated_at_ms`,
		managerConfigKey,
		string(data),
		cfg.UpdatedAtMS,
	)
	return err
}

func (r *repository) LoadManagerConfig(ctx context.Context) (model.ManagerConfig, bool, error) {
	var raw string
	err := r.db.QueryRowContext(ctx, `select value from settings where key = ?`, managerConfigKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return model.ManagerConfig{}, false, nil
	}
	if err != nil {
		return model.ManagerConfig{}, false, err
	}
	var cfg model.ManagerConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return model.ManagerConfig{}, false, err
	}
	cfg, err = r.unprotectManagerConfig(cfg)
	if err != nil {
		return model.ManagerConfig{}, false, err
	}
	return cfg, true, nil
}

func (r *repository) SaveAutomationSettings(ctx context.Context, settings model.AutomationSettings) (model.AutomationSettings, error) {
	settings.UpdatedAtMS = time.Now().UnixMilli()
	data, err := json.Marshal(settings)
	if err != nil {
		return model.AutomationSettings{}, err
	}
	_, err = r.db.ExecContext(
		ctx,
		`insert into settings(key, value, updated_at_ms)
		 values(?, ?, ?)
		 on conflict(key) do update set value = excluded.value, updated_at_ms = excluded.updated_at_ms`,
		automationSettingsKey,
		string(data),
		settings.UpdatedAtMS,
	)
	if err != nil {
		return model.AutomationSettings{}, err
	}
	return settings, nil
}

func (r *repository) LoadAutomationSettings(ctx context.Context) (model.AutomationSettings, bool, error) {
	var raw string
	err := r.db.QueryRowContext(ctx, `select value from settings where key = ?`, automationSettingsKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return model.AutomationSettings{}, false, nil
	}
	if err != nil {
		return model.AutomationSettings{}, false, err
	}
	var settings model.AutomationSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return model.AutomationSettings{}, false, err
	}
	return settings, true, nil
}

func (r *repository) SaveAdminCredential(ctx context.Context, credential model.AdminCredential) error {
	if credential.KeyHash == "" || credential.Salt == "" {
		return errors.New("admin credential is required")
	}
	data, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(
		ctx,
		`insert into settings(key, value, updated_at_ms)
		 values(?, ?, ?)
		 on conflict(key) do update set value = excluded.value, updated_at_ms = excluded.updated_at_ms`,
		adminCredentialKey,
		string(data),
		time.Now().UnixMilli(),
	)
	return err
}

func (r *repository) LoadAdminCredential(ctx context.Context) (model.AdminCredential, bool, error) {
	var raw string
	err := r.db.QueryRowContext(ctx, `select value from settings where key = ?`, adminCredentialKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return model.AdminCredential{}, false, nil
	}
	if err != nil {
		return model.AdminCredential{}, false, err
	}
	var credential model.AdminCredential
	if err := json.Unmarshal([]byte(raw), &credential); err != nil {
		return model.AdminCredential{}, false, err
	}
	return credential, true, nil
}

func (r *repository) SaveBootstrapState(ctx context.Context, state model.BootstrapState) error {
	state.UpdatedAtMS = time.Now().UnixMilli()
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(
		ctx,
		`insert into settings(key, value, updated_at_ms)
		 values(?, ?, ?)
		 on conflict(key) do update set value = excluded.value, updated_at_ms = excluded.updated_at_ms`,
		bootstrapStateKey,
		string(data),
		state.UpdatedAtMS,
	)
	return err
}

func (r *repository) LoadBootstrapState(ctx context.Context) (model.BootstrapState, bool, error) {
	var raw string
	err := r.db.QueryRowContext(ctx, `select value from settings where key = ?`, bootstrapStateKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return model.BootstrapState{}, false, nil
	}
	if err != nil {
		return model.BootstrapState{}, false, err
	}
	var state model.BootstrapState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return model.BootstrapState{}, false, err
	}
	return state, true, nil
}

func (r *repository) SaveBootstrapCredential(ctx context.Context, credential model.BootstrapCredential) error {
	if credential.TokenHash == "" || credential.Salt == "" {
		return errors.New("bootstrap credential is required")
	}
	data, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(
		ctx,
		`insert into settings(key, value, updated_at_ms)
		 values(?, ?, ?)
		 on conflict(key) do update set value = excluded.value, updated_at_ms = excluded.updated_at_ms`,
		bootstrapCredentialKey,
		string(data),
		time.Now().UnixMilli(),
	)
	return err
}

func (r *repository) LoadBootstrapCredential(ctx context.Context) (model.BootstrapCredential, bool, error) {
	var raw string
	err := r.db.QueryRowContext(ctx, `select value from settings where key = ?`, bootstrapCredentialKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return model.BootstrapCredential{}, false, nil
	}
	if err != nil {
		return model.BootstrapCredential{}, false, err
	}
	var credential model.BootstrapCredential
	if err := json.Unmarshal([]byte(raw), &credential); err != nil {
		return model.BootstrapCredential{}, false, err
	}
	return credential, true, nil
}

func (r *repository) SaveDeploymentState(ctx context.Context, state model.DeploymentState) error {
	state.UpdatedAtMS = time.Now().UnixMilli()
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(
		ctx,
		`insert into settings(key, value, updated_at_ms)
		 values(?, ?, ?)
		 on conflict(key) do update set value = excluded.value, updated_at_ms = excluded.updated_at_ms`,
		deploymentStateKey,
		string(data),
		state.UpdatedAtMS,
	)
	return err
}

func (r *repository) LoadDeploymentState(ctx context.Context) (model.DeploymentState, bool, error) {
	var raw string
	err := r.db.QueryRowContext(ctx, `select value from settings where key = ?`, deploymentStateKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return model.DeploymentState{}, false, nil
	}
	if err != nil {
		return model.DeploymentState{}, false, err
	}
	var state model.DeploymentState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return model.DeploymentState{}, false, err
	}
	return state, true, nil
}

func (r *repository) HasHistoricalData(ctx context.Context) (bool, error) {
	tables := []string{"usage_events", "dead_letter_events", "model_prices", "api_key_aliases"}
	for _, table := range tables {
		var count int64
		if err := r.db.QueryRowContext(ctx, `select count(*) from `+table).Scan(&count); err != nil {
			return false, err
		}
		if count > 0 {
			return true, nil
		}
	}
	var settingsCount int64
	if err := r.db.QueryRowContext(
		ctx,
		`select count(*) from settings where key in ('setup', ?)`,
		managerConfigKey,
	).Scan(&settingsCount); err != nil {
		return false, err
	}
	return settingsCount > 0, nil
}

func (r *repository) protectSetup(setup model.Setup) (model.Setup, error) {
	if r.protector == nil {
		return setup, nil
	}
	value, err := r.protector.ProtectString(setup.ManagementKey)
	if err != nil {
		return model.Setup{}, err
	}
	setup.ManagementKey = value
	return setup, nil
}

func (r *repository) unprotectSetup(setup model.Setup) (model.Setup, error) {
	if r.protector == nil {
		return setup, nil
	}
	value, err := r.protector.UnprotectString(setup.ManagementKey)
	if err != nil {
		return model.Setup{}, err
	}
	setup.ManagementKey = value
	return setup, nil
}

func (r *repository) protectManagerConfig(cfg model.ManagerConfig) (model.ManagerConfig, error) {
	if r.protector == nil {
		return cfg, nil
	}
	value, err := r.protector.ProtectString(cfg.CPAConnection.ManagementKey)
	if err != nil {
		return model.ManagerConfig{}, err
	}
	cfg.CPAConnection.ManagementKey = value
	return cfg, nil
}

func (r *repository) unprotectManagerConfig(cfg model.ManagerConfig) (model.ManagerConfig, error) {
	if r.protector == nil {
		return cfg, nil
	}
	value, err := r.protector.UnprotectString(cfg.CPAConnection.ManagementKey)
	if err != nil {
		return model.ManagerConfig{}, err
	}
	cfg.CPAConnection.ManagementKey = value
	return cfg, nil
}
