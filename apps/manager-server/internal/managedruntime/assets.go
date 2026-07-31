package managedruntime

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/model"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/security"
)

type Secrets struct {
	RuntimeKey       string
	CPAManagementKey string
}

func EnsureAssets(cfg Config) (Secrets, error) {
	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "runtime"), 0o700); err != nil {
		return Secrets{}, err
	}
	runtimeKey, err := loadOrCreateSecret(cfg.RuntimeKeyPath, "cpamp_runtime_")
	if err != nil {
		return Secrets{}, err
	}
	secrets := Secrets{RuntimeKey: runtimeKey}
	if cfg.DeploymentMode != model.DeploymentModeIntegrated {
		secrets.CPAManagementKey = strings.TrimSpace(os.Getenv("CPA_MANAGEMENT_KEY"))
		return secrets, nil
	}
	managementKey := strings.TrimSpace(os.Getenv("CPA_MANAGEMENT_KEY"))
	if managementKey == "" {
		if value, readErr := os.ReadFile(cfg.CPAManagementKeyPath); readErr == nil {
			managementKey = strings.TrimSpace(string(value))
		} else if !errors.Is(readErr, os.ErrNotExist) {
			return Secrets{}, readErr
		}
	}
	if managementKey == "" {
		if _, err := os.Stat(cfg.CPAConfigPath); err == nil {
			return Secrets{}, errors.New("existing integrated CPA config requires CPA_MANAGEMENT_KEY or a runtime CPA key file")
		} else if !errors.Is(err, os.ErrNotExist) {
			return Secrets{}, err
		}
		managementKey, err = security.GenerateServiceSecret("cpa_")
		if err != nil {
			return Secrets{}, err
		}
	}
	if err := writeSecret(cfg.CPAManagementKeyPath, managementKey); err != nil {
		return Secrets{}, err
	}
	if err := ensureCPAConfig(cfg, managementKey); err != nil {
		return Secrets{}, err
	}
	secrets.CPAManagementKey = managementKey
	return secrets, nil
}

func loadOrCreateSecret(path string, prefix string) (string, error) {
	if data, err := os.ReadFile(path); err == nil {
		value := strings.TrimSpace(string(data))
		if value == "" {
			return "", fmt.Errorf("secret file %s is empty", path)
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return "", err
		}
		return value, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	value, err := security.GenerateServiceSecret(prefix)
	if err != nil {
		return "", err
	}
	if err := writeSecret(path, value); err != nil {
		return "", err
	}
	return value, nil
}

func writeSecret(path string, value string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".secret-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.WriteString(strings.TrimSpace(value) + "\n"); err != nil {
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
	return replaceRuntimeFile(temporaryPath, path)
}

func ensureCPAConfig(cfg Config, managementKey string) error {
	if _, err := os.Stat(cfg.CPAConfigPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	host, port, err := splitHostPort(cfg.CPAAddr)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.CPAAuthDir, 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.CPAConfigPath), 0o700); err != nil {
		return err
	}
	content := fmt.Sprintf(
		"host: %s\nport: %d\nremote-management:\n  allow-remote: false\n  secret-key: %s\n  disable-control-panel: true\nauth-dir: %s\nusage-statistics-enabled: true\nredis-usage-queue-retention-seconds: 60\n",
		strconv.Quote(host),
		port,
		strconv.Quote(managementKey),
		strconv.Quote(cfg.CPAAuthDir),
	)
	temporary, err := os.CreateTemp(filepath.Dir(cfg.CPAConfigPath), ".config-*.yaml")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.WriteString(content); err != nil {
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
	return replaceRuntimeFile(temporaryPath, cfg.CPAConfigPath)
}

func splitHostPort(value string) (string, int, error) {
	host, rawPort, err := netSplitHostPort(value)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}

var netSplitHostPort = func(value string) (string, string, error) {
	return net.SplitHostPort(value)
}
