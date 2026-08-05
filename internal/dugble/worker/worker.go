package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// FailurePolicy documents how the worker reacts when a component exits.
type FailurePolicy string

const (
	// FailFast stops every component when any component exits unexpectedly.
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
		if strings.TrimSpace(component.Name) == "" {
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

func (supervisor *Supervisor) Policy() FailurePolicy { return supervisor.policy }

func (supervisor *Supervisor) Ready() bool {
	if supervisor == nil {
		return false
	}
	supervisor.mu.RLock()
	defer supervisor.mu.RUnlock()
	for _, state := range supervisor.states {
		if state.Status != StatusRunning {
			return false
		}
	}
	return len(supervisor.states) > 0
}

func (supervisor *Supervisor) Snapshot() map[string]ComponentState {
	if supervisor == nil {
		return nil
	}
	supervisor.mu.RLock()
	defer supervisor.mu.RUnlock()
	states := make(map[string]ComponentState, len(supervisor.states))
	for name, state := range supervisor.states {
		states[name] = state
	}
	return states
}

func (supervisor *Supervisor) Run(ctx context.Context, shutdownTimeout time.Duration) error {
	if supervisor == nil {
		return errors.New("worker supervisor is not configured")
	}
	if ctx == nil {
		return errors.New("worker supervisor context is required")
	}
	if shutdownTimeout <= 0 {
		shutdownTimeout = 30 * time.Second
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	type result struct {
		name string
		err  error
	}
	results := make(chan result, len(supervisor.components))
	for _, component := range supervisor.components {
		supervisor.setState(component.Name, ComponentState{Status: StatusRunning})
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
		runErr = supervisor.recordUnexpectedExit(componentResult.name, componentResult.err, runCtx)
		cancel()
	}

	supervisor.markRunningAs(StatusStopping)
	timer := time.NewTimer(shutdownTimeout)
	defer timer.Stop()
	for completed < len(supervisor.components) {
		select {
		case componentResult := <-results:
			completed++
			if componentResult.err != nil && !errors.Is(componentResult.err, context.Canceled) {
				supervisor.setState(componentResult.name, ComponentState{Status: StatusFailed, Error: componentResult.err.Error()})
				runErr = errors.Join(runErr, fmt.Errorf("run %s: %w", componentResult.name, componentResult.err))
			} else {
				supervisor.setState(componentResult.name, ComponentState{Status: StatusStopped})
			}
		case <-timer.C:
			supervisor.markIncompleteAsFailed("shutdown timeout")
			return errors.Join(runErr, fmt.Errorf("worker components did not stop within %s", shutdownTimeout))
		}
	}
	return runErr
}

func (supervisor *Supervisor) markIncompleteAsFailed(message string) {
	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()
	for name, state := range supervisor.states {
		if state.Status != StatusStopped && state.Status != StatusFailed {
			state.Status = StatusFailed
			state.Error = message
			supervisor.states[name] = state
		}
	}
}

func (supervisor *Supervisor) recordUnexpectedExit(name string, err error, runCtx context.Context) error {
	if runCtx.Err() != nil || errors.Is(err, context.Canceled) {
		supervisor.setState(name, ComponentState{Status: StatusStopped})
		return nil
	}
	if err == nil {
		err = errors.New("component stopped unexpectedly")
	}
	supervisor.setState(name, ComponentState{Status: StatusFailed, Error: err.Error()})
	return fmt.Errorf("run %s: %w", name, err)
}

func (supervisor *Supervisor) setState(name string, state ComponentState) {
	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()
	supervisor.states[name] = state
}

func (supervisor *Supervisor) markRunningAs(status ComponentStatus) {
	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()
	for name, state := range supervisor.states {
		if state.Status == StatusRunning || state.Status == StatusStarting {
			state.Status = status
			supervisor.states[name] = state
		}
	}
}

// Worker owns the supervised background components and their process lifecycle.
type Worker struct {
	supervisor    *Supervisor
	healthAddress string
	metricsPath   string
}

func New(supervisor *Supervisor, healthAddress, metricsPath string) (*Worker, error) {
	if supervisor == nil {
		return nil, errors.New("worker supervisor is required")
	}
	return &Worker{
		supervisor:    supervisor,
		healthAddress: strings.TrimSpace(healthAddress),
		metricsPath:   strings.TrimSpace(metricsPath),
	}, nil
}

func (worker *Worker) Run(ctx context.Context) error {
	if worker == nil || worker.supervisor == nil {
		return errors.New("worker is not configured")
	}
	if ctx == nil {
		return errors.New("worker context is required")
	}
	slog.Info(
		"worker starting",
		"failure_policy", worker.supervisor.Policy(),
		"health_address", worker.healthAddress,
		"metrics_path", worker.metricsPath,
	)
	if err := worker.supervisor.Run(ctx, 30*time.Second); err != nil {
		return err
	}
	slog.Info("worker stopped")
	return nil
}

type dependency interface {
	Ping(context.Context) error
}

func NewHealthHandler(postgres dependency, jetstream dependency, supervisor *Supervisor) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(response http.ResponseWriter, _ *http.Request) {
		writeJSON(response, http.StatusOK, map[string]any{"status": "ok", "service": "dugble-worker"})
	})
	mux.HandleFunc("GET /ready", func(response http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithTimeout(request.Context(), 3*time.Second)
		defer cancel()
		checks := map[string]string{}
		postgresReady := checkDependency(ctx, postgres, checks, "postgres")
		jetstreamReady := checkDependency(ctx, jetstream, checks, "jetstream")
		componentsReady := supervisor != nil && supervisor.Ready()
		if componentsReady {
			checks["components"] = "ok"
		} else {
			checks["components"] = "unavailable"
		}
		status := http.StatusOK
		readiness := "ready"
		if !postgresReady || !jetstreamReady || !componentsReady {
			status = http.StatusServiceUnavailable
			readiness = "not_ready"
		}
		result := map[string]any{"status": readiness, "checks": checks}
		if supervisor != nil {
			result["failure_policy"] = supervisor.Policy()
			result["components"] = supervisor.Snapshot()
		}
		writeJSON(response, status, result)
	})
	return mux
}

func checkDependency(ctx context.Context, value dependency, checks map[string]string, name string) bool {
	if value == nil {
		checks[name] = "unconfigured"
		return false
	}
	if err := value.Ping(ctx); err != nil {
		checks[name] = "unavailable"
		return false
	}
	checks[name] = "ok"
	return true
}

func writeJSON(response http.ResponseWriter, status int, body any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(body)
}
