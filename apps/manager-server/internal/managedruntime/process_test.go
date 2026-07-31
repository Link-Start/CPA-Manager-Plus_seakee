package managedruntime

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/model"
)

func TestProcessSupervisorStopsChildWhenRunningStateCannotPersist(t *testing.T) {
	state, err := OpenStateStore(
		filepath.Join(t.TempDir(), "state.json"),
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	invalidTarget := filepath.Join(t.TempDir(), "state-directory")
	if err := os.Mkdir(invalidTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	state.path = invalidTarget
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	supervisor := NewProcessSupervisor(state, &output)
	err = supervisor.runOnce(t.Context(), ProcessSpec{
		Name:   "cpamp",
		Binary: executable,
		Args:   []string{"-test.run=TestManagedRuntimeHelperProcess"},
		Env:    map[string]string{"CPAMP_MANAGED_RUNTIME_HELPER": "1"},
	}, make(chan struct{}))
	if err == nil || !strings.Contains(err.Error(), "persist running state for cpamp") {
		t.Fatalf("runOnce() error = %v, output = %q", err, output.String())
	}
	supervisor.mu.Lock()
	active := supervisor.active["cpamp"]
	supervisor.mu.Unlock()
	if active != nil {
		t.Fatalf("child process remained active after persistence failure: pid=%d", active.Process.Pid)
	}
}

func TestManagedRuntimeHelperProcess(t *testing.T) {
	if os.Getenv("CPAMP_MANAGED_RUNTIME_HELPER") != "1" {
		return
	}
	for {
		time.Sleep(time.Second)
	}
}

func TestManagedRuntimeShutdownBudgetsAllowChildCleanup(t *testing.T) {
	// CPA reserves up to 30 seconds for its own graceful shutdown. Keep a margin
	// before the supervisor forces termination, then a second margin for the
	// runtime listeners and process bookkeeping to drain.
	if managedProcessGracefulStopTimeout <= 30*time.Second {
		t.Fatalf("managed process graceful stop timeout = %s, want > 30s", managedProcessGracefulStopTimeout)
	}
	if runtimeShutdownTimeout <= managedProcessGracefulStopTimeout {
		t.Fatalf(
			"runtime shutdown timeout = %s, want > managed process timeout %s",
			runtimeShutdownTimeout,
			managedProcessGracefulStopTimeout,
		)
	}
}

func TestMergeEnvironmentKeepsOverridesStable(t *testing.T) {
	got := mergeEnvironment(
		[]string{"A=one", "B=two"},
		map[string]string{"B": "override", "C": "three"},
	)
	joined := strings.Join(got, ",")
	if !strings.Contains(joined, "A=one") || !strings.Contains(joined, "B=override") || !strings.Contains(joined, "C=three") {
		t.Fatalf("merged environment = %v", got)
	}
}
