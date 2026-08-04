package session

import "testing"

func TestValidateSessionID(t *testing.T) {
	if got, err := validateSessionID(" session-id "); err != nil || got != "session-id" {
		t.Fatalf("validateSessionID() = %q, %v", got, err)
	}
	if _, err := validateSessionID(" "); err == nil {
		t.Fatal("validateSessionID() accepted an empty id")
	}
}
