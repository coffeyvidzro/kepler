package attempt

import "testing"

func TestParseChannel(t *testing.T) {
	t.Parallel()

	channel, err := ParseChannel(" EMAIL ")
	if err != nil {
		t.Fatalf("ParseChannel() error = %v", err)
	}
	if channel != ChannelEmail {
		t.Fatalf("ParseChannel() = %q, want %q", channel, ChannelEmail)
	}

	if _, err := ParseChannel("push"); err == nil {
		t.Fatal("ParseChannel() error = nil for unsupported channel")
	}
}
