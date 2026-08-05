package routing

import "testing"

func TestDefaultConfigValidates(t *testing.T) {
	t.Parallel()
	if err := DefaultConfig().Validate(); err != nil {
		t.Fatalf("DefaultConfig().Validate() error = %v", err)
	}
}
