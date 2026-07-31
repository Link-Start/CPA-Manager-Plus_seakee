package cpa

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/protocol"
)

type UsageConfig struct {
	UsageStatisticsEnabled          bool `json:"usageStatisticsEnabled"`
	RedisUsageQueueRetentionSeconds int  `json:"redisUsageQueueRetentionSeconds"`
	RetentionSourceDefault          bool `json:"retentionSourceDefault"`
}

type ManagementConfig struct {
	UsageConfig
	ProxyURL string `json:"proxyUrl,omitempty"`
}

type HTTPStatusError struct {
	Operation  string
	StatusCode int
	Status     string
}

var ErrUsageQueueRetentionInvalid = errors.New("CPA redis-usage-queue-retention-seconds must be greater than 0")

type PollIntervalExceedsRetentionError struct {
	PollIntervalMS   int
	RetentionSeconds int
}

func (e *PollIntervalExceedsRetentionError) Error() string {
	return fmt.Sprintf(
		"pollIntervalMs must be less than or equal to CPA redis-usage-queue-retention-seconds (%d seconds)",
		e.RetentionSeconds,
	)
}

func (e *HTTPStatusError) Error() string {
	return e.Operation + " failed: " + e.Status
}

func ValidateManagementAPI(ctx context.Context, baseURL string, key string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, NormalizeBaseURL(baseURL)+"/v0/management/config", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set(protocol.DirectCPARequestHeader, "1")
	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}
	return &HTTPStatusError{
		Operation:  "management API validation",
		StatusCode: res.StatusCode,
		Status:     res.Status,
	}
}

func FetchUsageConfig(ctx context.Context, baseURL string, key string) (UsageConfig, error) {
	cfg, err := FetchManagementConfig(ctx, baseURL, key)
	if err != nil {
		return UsageConfig{}, err
	}
	return cfg.UsageConfig, nil
}

func FetchManagementConfig(ctx context.Context, baseURL string, key string) (ManagementConfig, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, NormalizeBaseURL(baseURL)+"/v0/management/config", nil)
	if err != nil {
		return ManagementConfig{}, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set(protocol.DirectCPARequestHeader, "1")
	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return ManagementConfig{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return ManagementConfig{}, &HTTPStatusError{
			Operation:  "management API config request",
			StatusCode: res.StatusCode,
			Status:     res.Status,
		}
	}

	var raw map[string]any
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		return ManagementConfig{}, err
	}
	usageEnabled := readBoolField(raw, "usage-statistics-enabled", "usageStatisticsEnabled")
	retention, hasRetention := readIntField(raw, "redis-usage-queue-retention-seconds", "redisUsageQueueRetentionSeconds")
	if !hasRetention {
		retention = 60
	}
	return ManagementConfig{
		UsageConfig: UsageConfig{
			UsageStatisticsEnabled:          usageEnabled,
			RedisUsageQueueRetentionSeconds: retention,
			RetentionSourceDefault:          !hasRetention,
		},
		ProxyURL: readStringField(raw, "proxy-url", "proxyUrl", "proxy_url"),
	}, nil
}

func SetUsageStatisticsEnabled(ctx context.Context, baseURL string, key string, enabled bool) error {
	payload := map[string]bool{"value": enabled}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPut,
		NormalizeBaseURL(baseURL)+"/v0/management/usage-statistics-enabled",
		strings.NewReader(string(data)),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(protocol.DirectCPARequestHeader, "1")
	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}
	return &HTTPStatusError{
		Operation:  "enable CPA usage statistics",
		StatusCode: res.StatusCode,
		Status:     res.Status,
	}
}

func ValidateCollectorConfig(ctx context.Context, baseURL string, key string, pollIntervalMS int) error {
	usageCfg, err := FetchUsageConfig(ctx, baseURL, key)
	if err != nil {
		return err
	}
	retentionMS := usageCfg.RedisUsageQueueRetentionSeconds * 1000
	if retentionMS <= 0 {
		return ErrUsageQueueRetentionInvalid
	}
	if pollIntervalMS > retentionMS {
		return &PollIntervalExceedsRetentionError{
			PollIntervalMS:   pollIntervalMS,
			RetentionSeconds: usageCfg.RedisUsageQueueRetentionSeconds,
		}
	}
	return nil
}

func NormalizeBaseURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = "http://" + value
	}
	value = strings.TrimRight(value, "/")
	value = strings.TrimSuffix(value, "/v0/management")
	value = strings.TrimSuffix(value, "/v0")
	return value
}

func readBoolField(raw map[string]any, keys ...string) bool {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case bool:
			return typed
		case string:
			normalized := strings.ToLower(strings.TrimSpace(typed))
			return normalized == "1" || normalized == "true" || normalized == "yes" || normalized == "on"
		}
	}
	return false
}

func readIntField(raw map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return int(typed), true
		case int:
			return typed, true
		case json.Number:
			parsed, err := strconv.Atoi(typed.String())
			return parsed, err == nil
		case string:
			parsed, err := strconv.Atoi(strings.TrimSpace(typed))
			return parsed, err == nil
		}
	}
	return 0, false
}

func readStringField(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok || value == nil {
			continue
		}
		return strings.TrimSpace(fmt.Sprint(value))
	}
	return ""
}
