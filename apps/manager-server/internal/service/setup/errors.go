package setup

import (
	"errors"
	"net/http"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/problem"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/security"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/service/cpa"
)

func cpaConnectionRequiredError() error {
	return problem.New(
		"setup_cpa_connection_required",
		"cpaBaseUrl and managementKey are required",
		http.StatusBadRequest,
		"cpa-connection-required",
	)
}

func setupEnvironmentManagedError() error {
	return problem.New(
		"setup_env_managed",
		"setup is managed by environment variables",
		http.StatusConflict,
		"setup-managed-by-environment",
	)
}

func existingManagementKeyError() error {
	return problem.New(
		"invalid_existing_management_key",
		"invalid management key for existing setup",
		http.StatusUnauthorized,
		"cpa-management-key-invalid",
	)
}

func cpaValidationError(err error) error {
	var statusErr *cpa.HTTPStatusError
	if errors.As(err, &statusErr) && (statusErr.StatusCode == http.StatusUnauthorized || statusErr.StatusCode == http.StatusForbidden) {
		return problem.Wrap(
			err,
			"setup_cpa_management_key_invalid",
			"CPA management key validation failed",
			http.StatusUnauthorized,
			"cpa-management-key-invalid",
		)
	}
	return problem.Wrap(
		err,
		"setup_cpa_unreachable",
		"CPA management API validation failed",
		http.StatusBadGateway,
		"cpa-management-api-unreachable",
	)
}

func collectorConfigValidationError(err error) error {
	if errors.Is(err, cpa.ErrUsageQueueRetentionInvalid) {
		return problem.Wrap(
			err,
			"cpa_usage_retention_invalid",
			err.Error(),
			http.StatusBadRequest,
			"cpa-usage-retention-invalid",
		)
	}
	var pollErr *cpa.PollIntervalExceedsRetentionError
	if errors.As(err, &pollErr) {
		return problem.WithDetails(problem.Wrap(
			err,
			"poll_interval_exceeds_retention",
			err.Error(),
			http.StatusBadRequest,
			"poll-interval-exceeds-retention",
		), map[string]any{
			"pollIntervalMs":   pollErr.PollIntervalMS,
			"retentionSeconds": pollErr.RetentionSeconds,
		})
	}
	var statusErr *cpa.HTTPStatusError
	if errors.As(err, &statusErr) && (statusErr.StatusCode == http.StatusUnauthorized || statusErr.StatusCode == http.StatusForbidden) {
		return cpaValidationError(err)
	}
	return problem.Wrap(
		err,
		"management_api_config_failed",
		"failed to read CPA usage configuration",
		http.StatusBadGateway,
		"cpa-usage-config-unavailable",
	)
}

func usageStatisticsEnableError(err error) error {
	return problem.Wrap(
		err,
		"enable_cpa_usage_statistics_failed",
		"failed to enable CPA usage statistics",
		http.StatusBadGateway,
		"enable-cpa-usage-statistics-failed",
	)
}

func adminKeyValidationError(err error) error {
	code := "setup_admin_key_policy"
	anchor := "admin-key-policy"
	if errors.Is(err, security.ErrAdminKeyTooShort) {
		code = "setup_admin_key_too_short"
		anchor = "admin-key-too-short"
	}
	return problem.Wrap(err, code, err.Error(), http.StatusBadRequest, anchor)
}

func adminAlreadyInitializedError() error {
	return problem.New(
		"setup_admin_already_initialized",
		"admin credential is already initialized",
		http.StatusConflict,
		"admin-key-already-initialized",
	)
}

func invalidAdminAuthorizationError() error {
	return problem.New(
		"invalid_admin_key",
		"invalid admin key",
		http.StatusUnauthorized,
		"admin-key-invalid",
	)
}

func adminVerificationBusyError(err error) error {
	return problem.Wrap(
		err,
		"admin_verification_busy",
		"admin key verification is temporarily busy; retry shortly",
		http.StatusTooManyRequests,
		"admin-verification-busy",
	)
}

func bootstrapTokenUnavailableError() error {
	return problem.New(
		"setup_bootstrap_token_unavailable",
		"bootstrap token is not available; restart the service to issue a new token",
		http.StatusServiceUnavailable,
		"bootstrap-token-unavailable",
	)
}

func bootstrapTokenExpiredError() error {
	return problem.New(
		"setup_bootstrap_token_expired",
		"bootstrap token has expired; restart the service to issue a new token",
		http.StatusUnauthorized,
		"bootstrap-token-expired",
	)
}

func invalidBootstrapTokenError() error {
	return problem.New(
		"setup_bootstrap_token_invalid",
		"invalid bootstrap token",
		http.StatusUnauthorized,
		"bootstrap-token-invalid",
	)
}
