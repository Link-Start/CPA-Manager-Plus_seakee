package main

import (
	"strings"
	"testing"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/managedruntime"
)

func TestMergeReplacementRuntimeEnvironmentPreservesCPAComponent(t *testing.T) {
	environment := mergeReplacementRuntimeEnvironment([]string{
		"PRESERVED=value",
		"CPA_MANAGER_RUNTIME_DELEGATED=0",
		managedruntime.RuntimeHandoffOperationEnv + "=update_old",
		"CPA_MANAGER_RUNTIME_CPA_BINARY=/old/cpa",
		"CPA_MANAGER_CPA_VERSION=v7.1.0",
	}, managedruntime.Config{
		CPABinary:  "/package/cli-proxy-api",
		CPAVersion: "v7.2.0",
	}, "update_new")

	values := map[string]string{}
	for _, entry := range environment {
		name, value, _ := strings.Cut(entry, "=")
		values[name] = value
	}
	for name, want := range map[string]string{
		"PRESERVED":                               "value",
		"CPA_MANAGER_RUNTIME_DELEGATED":           "1",
		managedruntime.RuntimeHandoffOperationEnv: "update_new",
		"CPA_MANAGER_RUNTIME_CPA_BINARY":          "/package/cli-proxy-api",
		"CPA_MANAGER_CPA_VERSION":                 "v7.2.0",
	} {
		if values[name] != want {
			t.Fatalf("%s = %q, want %q", name, values[name], want)
		}
	}
}
