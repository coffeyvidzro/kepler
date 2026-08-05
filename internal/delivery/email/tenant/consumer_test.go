package tenantprovision

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestValidateCommand(t *testing.T) {
	command := Command{
		EventID: uuid.New(), TenantID: uuid.New(), TeamID: uuid.New(),
		SchemaVersion: 1,
	}
	if err := ValidateCommand(command); err != nil {
		t.Fatalf("ValidateCommand() error = %v", err)
	}
	command.SchemaVersion = 2
	if err := ValidateCommand(command); err == nil {
		t.Fatal("ValidateCommand() error = nil for unsupported schema")
	}
}

func TestDefaultRetryBackOffReturnsCopy(t *testing.T) {
	policy := DefaultRetryBackOff()
	policy[0] = time.Hour
	if DefaultRetryBackOff()[0] == time.Hour {
		t.Fatal("caller mutated default retry policy")
	}
}

func TestNormalizeRetryBackOffExtendsLastDelay(t *testing.T) {
	got := normalizeRetryBackOff([]time.Duration{time.Second, 2 * time.Second}, 5)
	want := []time.Duration{time.Second, 2 * time.Second, 2 * time.Second, 2 * time.Second, 2 * time.Second}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("delay[%d] = %s, want %s", index, got[index], want[index])
		}
	}
}
