package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// FailurePolicy documents how the worker reacts when a component exits.
type FailurePolicy string

const (
	// FailFast stops every component when any component exits unexpectedly.
	// The process can then be restarted by its orchestrator as one healthy unit.
	FailFast FailurePolicy = "fail_fast"
)

type Component struct {
	Name string
	Run  func(context.Context) error
}

type ComponentStatus string

const (
	StatusStarting ComponentStatus = "starting"
	StatusRunning  ComponentStatus = "running"
	StatusStopping ComponentStatus = "stopping"
	StatusStopped  ComponentStatus = "stopped"
	StatusFailed   ComponentStatus = "failed"
)

type ComponentState struct {
	Status ComponentStatus `json:"status"`
	Error  string          `json:"error,omitempty"`
}

type Supervisor struct {
	policy     FailurePolicy
	components []Component

	mu     sync.RWMutex
	states map[string]ComponentState
}

func NewSupervisor(policy FailurePolicy, components ...Component) (*Supervisor, error) {
	if policy != FailFast {
		return nil, fmt.Errorf("unsupported worker failure policy %q", policy)
	}
	if len(components) == 0 {
		return nil, errors.New("worker supervisor requires at least one component")
	}

	states := make(map[string]ComponentState, len(components))
	for _, component := range components {
		if component.Name == "" {
			return nil, errors.New("worker component name is required")
		}
		if component.Run == nil {
			return nil, fmt.Errorf("worker component %q has no run function", component.Name)
		}
		if _, exists := states[component.Name]; exists {
			return nil, fmt.Errorf("duplicate worker component %q", component.Name)
		}
		states[component.Name] = ComponentState{Status: StatusStarting}
	}

	return &Supervisor{policy: policy, components: components, states: states}, nil
}

func (s *Supervisor) Policy() FailurePolicy { return s.policy }

func (s *Supervisor) Ready() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, state := range s.states {
		if state.Status != StatusRunning {
			return false
		}
	}
	return len(s.states) > 0
}

func (s *Supervisor) Snapshot() map[string]ComponentState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	states := make(map[string]ComponentState, len(s.states))
	for name, state := range s.states {
		states[name] = state
	}
	return states
}

func (s *Supervisor) Run(ctx context.Context, shutdownTimeout time.Duration) error {
	if shutdownTimeout <= 0 {
		shutdownTimeout = 30 * time.Second
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		name string
		err  error
	}
	results := make(chan result, len(s.components))
	for _, component := range s.components {
		s.setState(component.Name, ComponentState{Status: StatusRunning})
		go func(component Component) {
			results <- result{name: component.Name, err: component.Run(runCtx)}
		}(component)
	}

	completed := 0
	var runErr error
	select {
	case <-ctx.Done():
		cancel()
	case componentResult := <-results:
		completed++
		runErr = s.recordUnexpectedExit(componentResult.name, componentResult.err, runCtx)
		cancel()
	}

	s.markRunningAs(StatusStopping)
	timer := time.NewTimer(shutdownTimeout)
	defer timer.Stop()
	for completed < len(s.components) {
		select {
		case componentResult := <-results:
			completed++
			if componentResult.err != nil && !errors.Is(componentResult.err, context.Canceled) {
				s.setState(componentResult.name, ComponentState{Status: StatusFailed, Error: componentResult.err.Error()})
				runErr = errors.Join(runErr, fmt.Errorf("run %s: %w", componentResult.name, componentResult.err))
			} else {
				s.setState(componentResult.name, ComponentState{Status: StatusStopped})
			}
		case <-timer.C:
			s.markIncompleteAsFailed("shutdown timeout")
			return errors.Join(runErr, fmt.Errorf("worker components did not stop within %s", shutdownTimeout))
		}
	}
	return runErr
}

func (s *Supervisor) markIncompleteAsFailed(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for name, state := range s.states {
		if state.Status != StatusStopped && state.Status != StatusFailed {
			state.Status = StatusFailed
			state.Error = message
			s.states[name] = state
		}
	}
}

func (s *Supervisor) recordUnexpectedExit(name string, err error, runCtx context.Context) error {
	if runCtx.Err() != nil || errors.Is(err, context.Canceled) {
		s.setState(name, ComponentState{Status: StatusStopped})
		return nil
	}
	if err == nil {
		err = errors.New("component stopped unexpectedly")
	}
	s.setState(name, ComponentState{Status: StatusFailed, Error: err.Error()})
	return fmt.Errorf("run %s: %w", name, err)
}

func (s *Supervisor) setState(name string, state ComponentState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[name] = state
}

func (s *Supervisor) markRunningAs(status ComponentStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for name, state := range s.states {
		if state.Status == StatusRunning || state.Status == StatusStarting {
			state.Status = status
			s.states[name] = state
		}
	}
}
