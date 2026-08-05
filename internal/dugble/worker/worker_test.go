package worker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSupervisorFailFastCancelsOtherComponents(t *testing.T) {
	failure := errors.New("consumer failed")
	cancelled := make(chan struct{})
	supervisor, err := NewSupervisor(FailFast,
		Component{Name: "failing", Run: func(context.Context) error { return failure }},
		Component{Name: "peer", Run: func(ctx context.Context) error {
			<-ctx.Done()
			close(cancelled)
			return nil
		}},
	)
	if err != nil {
		t.Fatalf("NewSupervisor() error = %v", err)
	}

	err = supervisor.Run(context.Background(), time.Second)
	if err == nil || !strings.Contains(err.Error(), "run failing: consumer failed") {
		t.Fatalf("Run() error = %v, want failing component error", err)
	}
	select {
	case <-cancelled:
	default:
		t.Fatal("peer component was not cancelled")
	}
	if supervisor.Snapshot()["failing"].Status != StatusFailed {
		t.Fatalf("failing component state = %+v", supervisor.Snapshot()["failing"])
	}
}

func TestSupervisorGracefulCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	supervisor, err := NewSupervisor(FailFast, Component{Name: "component", Run: func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return nil
	}})
	if err != nil {
		t.Fatalf("NewSupervisor() error = %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- supervisor.Run(ctx, time.Second) }()
	<-started
	if !supervisor.Ready() {
		t.Fatal("supervisor should be ready while all components are running")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if supervisor.Snapshot()["component"].Status != StatusStopped {
		t.Fatalf("component state = %+v", supervisor.Snapshot()["component"])
	}
}

func TestNewSupervisorRejectsUnsupportedConfiguration(t *testing.T) {
	tests := []struct {
		name       string
		policy     FailurePolicy
		components []Component
	}{
		{name: "policy", policy: "restart", components: []Component{{Name: "component", Run: func(context.Context) error { return nil }}}},
		{name: "empty", policy: FailFast},
		{name: "duplicate", policy: FailFast, components: []Component{{Name: "same", Run: func(context.Context) error { return nil }}, {Name: "same", Run: func(context.Context) error { return nil }}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewSupervisor(test.policy, test.components...); err == nil {
				t.Fatal("NewSupervisor() error = nil")
			}
		})
	}
}
