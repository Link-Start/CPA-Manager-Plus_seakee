package managedruntime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/model"
	_ "modernc.org/sqlite"
)

type Config struct {
	DataDir              string
	UsageDBPath          string
	StatePath            string
	DeploymentMode       model.DeploymentMode
	PanelBasePath        string
	PanelBasePathSource  string
	GatewayAddrs         []string
	ManagerAddr          string
	ManagerURL           string
	ManagerBinary        string
	CPAAddr              string
	CPAURL               string
	CPAUpstreamEnvSet    bool
	CPABinary            string
	CPAConfigPath        string
	CPAAuthDir           string
	CPAManagementKeyPath string
	ControlAddr          string
	ControlURL           string
	RuntimeKeyPath       string
	ReleaseManifestURL   string
	ReleasePublicKey     string
	ReleaseDownloadDir   string
	ComponentDir         string
	CPAMPVersion         string
	CPAVersion           string
}

var (
	BuildVersion             = "dev"
	EmbeddedReleasePublicKey = ""
	EmbeddedCPAVersion       = ""
)

func LoadConfig() (Config, error) {
	dataDir := firstNonEmpty(os.Getenv("CPA_MANAGER_RUNTIME_DATA_DIR"), os.Getenv("USAGE_DATA_DIR"), "./data")
	dataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return Config{}, err
	}
	managerAddr := firstNonEmpty(os.Getenv("CPA_MANAGER_RUNTIME_MANAGER_ADDR"), "127.0.0.1:18318")
	cpaAddr := firstNonEmpty(os.Getenv("CPA_MANAGER_RUNTIME_CPA_ADDR"), "127.0.0.1:8317")
	controlAddr := firstNonEmpty(os.Getenv("CPA_MANAGER_RUNTIME_CONTROL_ADDR"), "127.0.0.1:18319")
	panelBasePathValue := strings.TrimSpace(os.Getenv("CPA_MANAGER_PANEL_BASE_PATH"))
	panelBasePathSource := model.PanelBasePathSourceRuntime
	if panelBasePathValue == "" {
		panelBasePathValue = "/management.html"
	} else {
		panelBasePathSource = model.PanelBasePathSourceEnvironment
	}
	panelBasePath, err := model.NormalizePanelBasePath(panelBasePathValue)
	if err != nil {
		return Config{}, err
	}
	executable, err := os.Executable()
	if err != nil {
		return Config{}, err
	}
	cpaBinary := strings.TrimSpace(os.Getenv("CPA_MANAGER_RUNTIME_CPA_BINARY"))
	if cpaBinary == "" {
		cpaBinary = findCPABinary(executable)
	}
	dbPath := firstNonEmpty(os.Getenv("USAGE_DB_PATH"), filepath.Join(dataDir, "usage.sqlite"))
	if !filepath.IsAbs(dbPath) {
		dbPath, err = filepath.Abs(dbPath)
		if err != nil {
			return Config{}, err
		}
	}
	cpaURL := strings.TrimSpace(os.Getenv("CPA_UPSTREAM_URL"))
	cpaUpstreamEnvSet := cpaURL != ""
	legacyCPAURL := ""
	if cpaURL == "" {
		legacyCPAURL, err = loadLegacyCPAUpstream(dbPath)
		if err != nil {
			return Config{}, err
		}
		cpaURL = legacyCPAURL
	}
	modeValue := strings.TrimSpace(os.Getenv("CPA_MANAGER_DEPLOYMENT_MODE"))
	mode := inferDeploymentMode(modeValue, cpaURL, legacyCPAURL, cpaBinary)
	if cpaURL == "" || mode == model.DeploymentModeIntegrated {
		cpaURL = httpURL(cpaAddr)
	}
	cfg := Config{
		DataDir:              dataDir,
		UsageDBPath:          dbPath,
		StatePath:            firstNonEmpty(os.Getenv("CPA_MANAGER_RUNTIME_STATE_PATH"), filepath.Join(dataDir, "runtime", "state.json")),
		DeploymentMode:       mode,
		PanelBasePath:        panelBasePath,
		PanelBasePathSource:  panelBasePathSource,
		GatewayAddrs:         splitCSV(firstNonEmpty(os.Getenv("CPA_MANAGER_GATEWAY_ADDRS"), "0.0.0.0:8137,0.0.0.0:18137,0.0.0.0:18317")),
		ManagerAddr:          managerAddr,
		ManagerURL:           httpURL(managerAddr),
		ManagerBinary:        firstNonEmpty(os.Getenv("CPA_MANAGER_RUNTIME_MANAGER_BINARY"), executable),
		CPAAddr:              cpaAddr,
		CPAURL:               cpaURL,
		CPAUpstreamEnvSet:    cpaUpstreamEnvSet,
		CPABinary:            cpaBinary,
		CPAConfigPath:        firstNonEmpty(os.Getenv("CPA_MANAGER_RUNTIME_CPA_CONFIG"), filepath.Join(dataDir, "cpa", "config.yaml")),
		CPAAuthDir:           firstNonEmpty(os.Getenv("CPA_MANAGER_RUNTIME_CPA_AUTH_DIR"), filepath.Join(dataDir, "cpa", "auths")),
		CPAManagementKeyPath: firstNonEmpty(os.Getenv("CPA_MANAGER_RUNTIME_CPA_KEY_PATH"), filepath.Join(dataDir, "runtime", "cpa-management.key")),
		ControlAddr:          controlAddr,
		ControlURL:           httpURL(controlAddr),
		RuntimeKeyPath:       firstNonEmpty(os.Getenv("CPA_MANAGER_RUNTIME_KEY_PATH"), filepath.Join(dataDir, "runtime", "control.key")),
		ReleaseManifestURL:   firstNonEmpty(os.Getenv("CPA_MANAGER_RELEASE_MANIFEST_URL"), "https://github.com/seakee/CPA-Manager-Plus/releases/latest/download/runtime-manifest.json"),
		ReleasePublicKey:     firstNonEmpty(os.Getenv("CPA_MANAGER_RELEASE_PUBLIC_KEY"), EmbeddedReleasePublicKey),
		ReleaseDownloadDir:   firstNonEmpty(os.Getenv("CPA_MANAGER_RUNTIME_DOWNLOAD_DIR"), filepath.Join(dataDir, "runtime", "downloads")),
		ComponentDir:         firstNonEmpty(os.Getenv("CPA_MANAGER_RUNTIME_COMPONENT_DIR"), filepath.Join(dataDir, "runtime", "components")),
		CPAMPVersion:         firstNonEmpty(os.Getenv("CPA_MANAGER_VERSION"), BuildVersion),
		CPAVersion:           firstNonEmpty(os.Getenv("CPA_MANAGER_CPA_VERSION"), EmbeddedCPAVersion),
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func inferDeploymentMode(explicit string, cpaURL string, legacyCPAURL string, cpaBinary string) model.DeploymentMode {
	if strings.TrimSpace(explicit) != "" {
		return model.NormalizeDeploymentMode(explicit)
	}
	if strings.TrimSpace(legacyCPAURL) != "" {
		return model.DeploymentModeExternal
	}
	if strings.TrimSpace(cpaURL) != "" {
		return model.DeploymentModeInstallerManaged
	}
	if strings.TrimSpace(cpaBinary) != "" {
		return model.DeploymentModeIntegrated
	}
	return model.DeploymentModeSlim
}

func loadLegacyCPAUpstream(dbPath string) (string, error) {
	info, err := os.Stat(dbPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("inspect legacy runtime database: %w", err)
	}
	if info.IsDir() || info.Size() == 0 {
		return "", nil
	}
	uriPath := filepath.ToSlash(dbPath)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	dsn := &url.URL{Scheme: "file", Path: uriPath}
	query := dsn.Query()
	query.Set("mode", "ro")
	query.Add("_pragma", "busy_timeout(1000)")
	query.Add("_pragma", "query_only(1)")
	dsn.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return "", fmt.Errorf("open legacy runtime database: %w", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	rows, err := db.QueryContext(ctx, `select key, value from settings where key in ('manager_config_v1', 'setup')`)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return "", nil
		}
		return "", fmt.Errorf("read legacy CPA connection: %w", err)
	}
	defer rows.Close()
	values := map[string]string{}
	for rows.Next() {
		var key string
		var value string
		if err := rows.Scan(&key, &value); err != nil {
			return "", fmt.Errorf("scan legacy CPA connection: %w", err)
		}
		values[key] = value
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("read legacy CPA connection rows: %w", err)
	}
	if raw := values["manager_config_v1"]; raw != "" {
		var config struct {
			CPAConnection struct {
				BaseURL string `json:"cpaBaseUrl"`
			} `json:"cpaConnection"`
		}
		if err := json.Unmarshal([]byte(raw), &config); err != nil {
			return "", fmt.Errorf("parse legacy manager config: %w", err)
		}
		if value := strings.TrimSpace(config.CPAConnection.BaseURL); value != "" {
			return value, nil
		}
	}
	if raw := values["setup"]; raw != "" {
		var setup struct {
			BaseURL string `json:"cpaBaseUrl"`
		}
		if err := json.Unmarshal([]byte(raw), &setup); err != nil {
			return "", fmt.Errorf("parse legacy setup: %w", err)
		}
		return strings.TrimSpace(setup.BaseURL), nil
	}
	return "", nil
}

func (c Config) Validate() error {
	if len(c.GatewayAddrs) == 0 {
		return errors.New("at least one gateway address is required")
	}
	for _, addr := range append(append([]string{}, c.GatewayAddrs...), c.ManagerAddr, c.ControlAddr) {
		if _, _, err := net.SplitHostPort(addr); err != nil {
			return err
		}
	}
	if c.DeploymentMode == model.DeploymentModeIntegrated && strings.TrimSpace(c.CPABinary) == "" {
		return errors.New("integrated deployment requires a CPA binary")
	}
	return nil
}

func findCPABinary(managerExecutable string) string {
	for _, candidate := range []string{
		filepath.Join(filepath.Dir(managerExecutable), "cli-proxy-api.exe"),
		filepath.Join(filepath.Dir(managerExecutable), "cpa.exe"),
		filepath.Join(filepath.Dir(managerExecutable), "cli-proxy-api"),
		filepath.Join(filepath.Dir(managerExecutable), "cpa"),
		"/usr/local/bin/cli-proxy-api",
	} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	if candidate, err := exec.LookPath("cli-proxy-api"); err == nil {
		return candidate
	}
	return ""
}

func httpURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + addr
	}
	if host == "0.0.0.0" || host == "" {
		host = "127.0.0.1"
	}
	if host == "::" {
		host = "::1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
