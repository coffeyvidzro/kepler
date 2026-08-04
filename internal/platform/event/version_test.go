package event

import "testing"

func TestParseVersion(t *testing.T) {
	version, err := ParseVersion(" 1 ")
	if err != nil {
		t.Fatalf("ParseVersion() error = %v", err)
	}
	if version != CurrentVersion {
		t.Fatalf("ParseVersion() = %q, want %q", version, CurrentVersion)
	}
	if _, err := ParseVersion("2"); err == nil {
		t.Fatal("ParseVersion() accepted an unsupported version")
	}
}
