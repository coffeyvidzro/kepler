package feedback

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestReconciliationDelay(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 0, want: 5 * time.Second},
		{attempt: 1, want: 5 * time.Second},
		{attempt: 2, want: 30 * time.Second},
		{attempt: 6, want: time.Hour},
		{attempt: 12, want: 72 * time.Hour},
		{attempt: 50, want: 72 * time.Hour},
	}
	for _, test := range tests {
		if got := reconciliationDelay(test.attempt); got != test.want {
			t.Fatalf("reconciliationDelay(%d) = %s, want %s", test.attempt, got, test.want)
		}
	}
}

func TestNormalizeReconcilerConfig(t *testing.T) {
	config := normalizeReconcilerConfig(ReconcilerConfig{})
	if config.PollInterval != 5*time.Second || config.BatchSize != 25 || config.Concurrency != 5 {
		t.Fatalf("unexpected defaults: %+v", config)
	}
	if config.LeaseDuration != 2*time.Minute || config.HandleTimeout != 30*time.Second {
		t.Fatalf("unexpected timeout defaults: %+v", config)
	}
}

func TestDurationSecondsHasOneSecondMinimum(t *testing.T) {
	if got := durationSeconds(100 * time.Millisecond); got != 1 {
		t.Fatalf("durationSeconds() = %d, want 1", got)
	}
	if got := durationSeconds(2 * time.Minute); got != 120 {
		t.Fatalf("durationSeconds() = %d, want 120", got)
	}
}

func TestTruncateReconciliationError(t *testing.T) {
	message := strings.Repeat("x", 2048)
	got := truncateReconciliationError(errors.New(message))
	if len(got) != 1024 {
		t.Fatalf("truncated error length = %d, want 1024", len(got))
	}
}
