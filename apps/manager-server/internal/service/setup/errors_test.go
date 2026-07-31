package setup

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/problem"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/service/cpa"
)

func TestCPAValidationErrorMapsAuthorizationAndConnectivitySeparately(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		code       string
		status     int
		docsAnchor string
	}{
		{
			name: "unauthorized",
			err: &cpa.HTTPStatusError{
				Operation:  "management API validation",
				StatusCode: http.StatusUnauthorized,
				Status:     "401 Unauthorized",
			},
			code:       "setup_cpa_management_key_invalid",
			status:     http.StatusUnauthorized,
			docsAnchor: "#cpa-management-key-invalid",
		},
		{
			name:       "unreachable",
			err:        errors.New("dial tcp: connection refused"),
			code:       "setup_cpa_unreachable",
			status:     http.StatusBadGateway,
			docsAnchor: "#cpa-management-api-unreachable",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			typed, ok := problem.As(cpaValidationError(test.err))
			if !ok {
				t.Fatal("error is not a problem error")
			}
			if typed.Code != test.code || typed.Status != test.status {
				t.Fatalf("problem = %#v", typed)
			}
			if len(typed.DocsURL) < len(test.docsAnchor) || typed.DocsURL[len(typed.DocsURL)-len(test.docsAnchor):] != test.docsAnchor {
				t.Fatalf("docs URL = %q", typed.DocsURL)
			}
		})
	}
}

func TestCollectorConfigValidationErrorMapsEachFailureToDocumentation(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		code       string
		status     int
		docsAnchor string
	}{
		{
			name:       "invalid retention",
			err:        cpa.ErrUsageQueueRetentionInvalid,
			code:       "cpa_usage_retention_invalid",
			status:     http.StatusBadRequest,
			docsAnchor: "#cpa-usage-retention-invalid",
		},
		{
			name: "poll interval exceeds retention",
			err: &cpa.PollIntervalExceedsRetentionError{
				PollIntervalMS:   61000,
				RetentionSeconds: 60,
			},
			code:       "poll_interval_exceeds_retention",
			status:     http.StatusBadRequest,
			docsAnchor: "#poll-interval-exceeds-retention",
		},
		{
			name:       "usage config unavailable",
			err:        errors.New("unexpected EOF"),
			code:       "management_api_config_failed",
			status:     http.StatusBadGateway,
			docsAnchor: "#cpa-usage-config-unavailable",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			typed, ok := problem.As(collectorConfigValidationError(test.err))
			if !ok {
				t.Fatal("error is not a problem error")
			}
			if typed.Code != test.code || typed.Status != test.status || !strings.HasSuffix(typed.DocsURL, test.docsAnchor) {
				t.Fatalf("problem = %#v", typed)
			}
		})
	}
}

func TestUsageStatisticsEnableErrorIncludesSolutionDocumentation(t *testing.T) {
	typed, ok := problem.As(usageStatisticsEnableError(errors.New("endpoint unavailable")))
	if !ok {
		t.Fatal("error is not a problem error")
	}
	if typed.Code != "enable_cpa_usage_statistics_failed" || typed.Status != http.StatusBadGateway ||
		!strings.HasSuffix(typed.DocsURL, "#enable-cpa-usage-statistics-failed") {
		t.Fatalf("problem = %#v", typed)
	}
}
