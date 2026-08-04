package expiry

import (
	"testing"
	"time"
)

func TestNormalizeConfigUsesSafeDefaults(t *testing.T) {
	config := normalizeConfig(Config{})
	if config.PollInterval != time.Second {
		t.Fatalf("PollInterval = %s, want 1s", config.PollInterval)
	}
	if config.BatchSize != 100 {
		t.Fatalf("BatchSize = %d, want 100", config.BatchSize)
	}
	if config.BatchTimeout != 15*time.Second {
		t.Fatalf("BatchTimeout = %s, want 15s", config.BatchTimeout)
	}
}

func TestNormalizeConfigPreservesConfiguredValues(t *testing.T) {
	configured := Config{PollInterval: 2 * time.Second, BatchSize: 25, BatchTimeout: 3 * time.Second}
	if actual := normalizeConfig(configured); actual != configured {
		t.Fatalf("normalizeConfig() = %+v, want %+v", actual, configured)
	}
}
