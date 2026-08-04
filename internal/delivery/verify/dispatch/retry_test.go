package dispatch

import (
	"testing"
	"time"
)

func TestNormalizeConsumerConfigUsesSafeDefaults(t *testing.T) {
	config := normalizeConsumerConfig(ConsumerConfig{})
	if config.Concurrency != 5 || config.MaxDeliver != 6 || config.AckWait != 2*time.Minute {
		t.Fatalf("unexpected defaults: %+v", config)
	}
	if len(config.RetryDelays) != 6 {
		t.Fatalf("retry delays = %d, want 6", len(config.RetryDelays))
	}
}
