//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	managedruntime "github.com/seakee/cpa-manager-plus/apps/manager-server/internal/managedruntime"
	"golang.org/x/sys/windows"
)

const replacementHandoffExitCode = 77
const replacementRuntimeGracefulStopTimeout = 45 * time.Second

func runReplacementRuntime(
	ctx context.Context,
	cfg managedruntime.Config,
	binary string,
	operationID string,
) error {
	if strings.TrimSpace(os.Getenv("CPA_MANAGER_RUNTIME_DELEGATED")) != "" {
		os.Exit(replacementHandoffExitCode)
	}

	currentBinary := binary
	currentOperationID := operationID
	for {
		err := runReplacementRuntimeOnce(ctx, cfg, currentBinary, currentOperationID)
		if ctx.Err() != nil {
			return nil
		}
		exitErr, handoff := err.(*exec.ExitError)
		if handoff && exitErr.ExitCode() == replacementHandoffExitCode {
			cfg.ManagerBinary = currentBinary
			target, resolveErr := managedruntime.CurrentRuntimeTarget(cfg)
			if resolveErr != nil {
				return fmt.Errorf("resolve replacement runtime after handoff: %w", resolveErr)
			}
			if strings.TrimSpace(target.Binary) == "" {
				return errors.New("replacement runtime requested a handoff without a target binary")
			}
			currentBinary = target.Binary
			currentOperationID = target.OperationID
			continue
		}

		if currentOperationID != "" {
			recovered, recoveryErr := managedruntime.RecoverPendingHandoff(
				cfg,
				currentOperationID,
				err,
			)
			if recoveryErr != nil {
				return errors.Join(err, recoveryErr)
			}
			cfg.ManagerBinary = currentBinary
			target, resolveErr := managedruntime.CurrentRuntimeTarget(cfg)
			if resolveErr != nil {
				return fmt.Errorf("resolve restored runtime after failed handoff: %w", resolveErr)
			}
			if strings.TrimSpace(target.Binary) != "" {
				log.Printf("runtime handoff failed; restored the previous managed runtime: %v", err)
				currentBinary = target.Binary
				currentOperationID = target.OperationID
				continue
			}
			if recovered {
				return errors.New("previous runtime binary is unavailable after failed handoff")
			}
		}

		handleReplacementExit(err)
		return nil
	}
}

func runReplacementRuntimeOnce(
	ctx context.Context,
	cfg managedruntime.Config,
	binary string,
	operationID string,
) error {
	cmd := exec.Command(binary, "runtime")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = replacementRuntimeEnvironment(cfg, operationID)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start replacement runtime: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		if err := windows.GenerateConsoleCtrlEvent(
			windows.CTRL_BREAK_EVENT,
			uint32(cmd.Process.Pid),
		); err != nil {
			_ = cmd.Process.Kill()
		}
		select {
		case <-done:
			return nil
		case <-time.After(replacementRuntimeGracefulStopTimeout):
			_ = cmd.Process.Kill()
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
		return nil
	}
}

func handleReplacementExit(err error) {
	if err == nil {
		return
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		os.Exit(exitErr.ExitCode())
	}
	log.Fatalf("run replacement runtime: %v", err)
}
