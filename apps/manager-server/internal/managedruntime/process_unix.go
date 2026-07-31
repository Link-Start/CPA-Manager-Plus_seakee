//go:build !windows

package managedruntime

import (
	"os"
	"os/exec"
)

func configureManagedProcess(_ *exec.Cmd) {}

func interruptManagedProcess(process *os.Process) error {
	return process.Signal(os.Interrupt)
}
