//go:build !windows

package main

import (
	"context"
	"syscall"

	managedruntime "github.com/seakee/cpa-manager-plus/apps/manager-server/internal/managedruntime"
)

func runReplacementRuntime(
	_ context.Context,
	cfg managedruntime.Config,
	binary string,
	operationID string,
) error {
	environment := replacementRuntimeEnvironment(cfg, operationID)
	return syscall.Exec(binary, []string{binary, "runtime"}, environment)
}
