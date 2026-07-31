package managedruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type ProcessSpec struct {
	Name    string
	Binary  string
	Version string
	Args    []string
	Env     map[string]string
	Dir     string
}

type ProcessSpecProvider func() ProcessSpec

var errProcessRestartRequested = errors.New("process restart requested")

const managedProcessGracefulStopTimeout = 35 * time.Second

type ProcessSupervisor struct {
	state   *StateStore
	output  io.Writer
	mu      sync.Mutex
	active  map[string]*exec.Cmd
	restart map[string]chan struct{}
}

func NewProcessSupervisor(state *StateStore, output io.Writer) *ProcessSupervisor {
	if output == nil {
		output = os.Stderr
	}
	return &ProcessSupervisor{
		state:   state,
		output:  output,
		active:  map[string]*exec.Cmd{},
		restart: map[string]chan struct{}{},
	}
}

func (s *ProcessSupervisor) Run(ctx context.Context, name string, provider ProcessSpecProvider) {
	restart := make(chan struct{}, 1)
	s.mu.Lock()
	s.restart[name] = restart
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.restart, name)
		s.mu.Unlock()
	}()
	backoff := time.Second
	for ctx.Err() == nil {
		spec := provider()
		spec.Name = name
		err := s.runOnce(ctx, spec, restart)
		if ctx.Err() != nil {
			return
		}
		if stateErr := s.state.UpdateComponent(name, func(component ComponentState) ComponentState {
			component.Status = "restarting"
			component.PID = 0
			component.Restarts++
			if err != nil && !errors.Is(err, errProcessRestartRequested) {
				component.LastError = err.Error()
			}
			return component
		}); stateErr != nil {
			_, _ = fmt.Fprintf(s.output, "persist %s restart state: %v\n", name, stateErr)
		}
		if errors.Is(err, errProcessRestartRequested) {
			backoff = time.Second
			continue
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (s *ProcessSupervisor) Restart(name string) error {
	s.mu.Lock()
	restart := s.restart[name]
	s.mu.Unlock()
	if restart == nil {
		return fmt.Errorf("process %s is not supervised", name)
	}
	select {
	case restart <- struct{}{}:
	default:
	}
	return nil
}

func (s *ProcessSupervisor) runOnce(ctx context.Context, spec ProcessSpec, restart <-chan struct{}) error {
	if strings.TrimSpace(spec.Binary) == "" {
		return errors.New("process binary is required")
	}
	cmd := exec.Command(spec.Binary, spec.Args...)
	configureManagedProcess(cmd)
	cmd.Dir = spec.Dir
	cmd.Env = mergeEnvironment(os.Environ(), spec.Env)
	cmd.Stdout = s.output
	cmd.Stderr = s.output
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", spec.Name, err)
	}
	s.mu.Lock()
	s.active[spec.Name] = cmd
	s.mu.Unlock()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	if err := s.state.UpdateComponent(spec.Name, func(component ComponentState) ComponentState {
		component.Status = "running"
		component.PID = cmd.Process.Pid
		component.BinaryPath = spec.Binary
		if strings.TrimSpace(spec.Version) != "" {
			component.Version = strings.TrimSpace(spec.Version)
		}
		component.LastError = ""
		component.StartedAtMS = time.Now().UnixMilli()
		return component
	}); err != nil {
		s.stopProcess(cmd, done)
		s.clearActive(spec.Name, cmd)
		return fmt.Errorf("persist running state for %s: %w", spec.Name, err)
	}
	select {
	case err := <-done:
		s.clearActive(spec.Name, cmd)
		if err == nil {
			return errors.New("process exited")
		}
		return fmt.Errorf("%s exited: %w", spec.Name, err)
	case <-restart:
		s.stopProcess(cmd, done)
		s.clearActive(spec.Name, cmd)
		return errProcessRestartRequested
	case <-ctx.Done():
		s.stopProcess(cmd, done)
		s.clearActive(spec.Name, cmd)
		if err := s.state.UpdateComponent(spec.Name, func(component ComponentState) ComponentState {
			component.Status = "stopped"
			component.PID = 0
			return component
		}); err != nil {
			_, _ = fmt.Fprintf(s.output, "persist %s stopped state: %v\n", spec.Name, err)
			return errors.Join(ctx.Err(), fmt.Errorf("persist stopped state for %s: %w", spec.Name, err))
		}
		return ctx.Err()
	}
}

func (s *ProcessSupervisor) stopProcess(cmd *exec.Cmd, done <-chan error) {
	if err := interruptManagedProcess(cmd.Process); err != nil {
		_ = cmd.Process.Kill()
	}
	timer := time.NewTimer(managedProcessGracefulStopTimeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		_ = cmd.Process.Kill()
		<-done
	}
}

func (s *ProcessSupervisor) clearActive(name string, cmd *exec.Cmd) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[name] == cmd {
		delete(s.active, name)
	}
}

func mergeEnvironment(current []string, overrides map[string]string) []string {
	values := make(map[string]string, len(current)+len(overrides))
	order := make([]string, 0, len(current)+len(overrides))
	for _, item := range current {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		if _, exists := values[key]; !exists {
			order = append(order, key)
		}
		values[key] = value
	}
	for key, value := range overrides {
		if _, exists := values[key]; !exists {
			order = append(order, key)
		}
		values[key] = value
	}
	result := make([]string, 0, len(values))
	for _, key := range order {
		result = append(result, key+"="+values[key])
	}
	return result
}
