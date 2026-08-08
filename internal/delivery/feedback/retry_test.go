package feedback

import (
	"testing"
	"time"
)

func TestRetryPolicyCapsBackoffAndAttempts(t *testing.T) {
	t.Parallel()

	policy := RetryPolicy{
		InitialDelay: time.Second,
		MaxDelay:     4 * time.Second,
		Multiplier:   2,
		MaxAttempts:  4,
	}
	for attempt, want := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 4 * time.Second} {
		delay, ok := policy.Delay(int32(attempt + 1))
		if !ok || delay != want {
			t.Fatalf("RetryPolicy.Delay(%d) = %v, %v; want %v, true", attempt+1, delay, ok, want)
		}
	}
	if _, ok := policy.Delay(5); ok {
		t.Fatal("RetryPolicy.Delay(5) ok = true after maximum attempts")
	}
}
