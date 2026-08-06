package dispatch

import (
	"strings"
	"testing"
	"time"

	"github.com/coffeyvidzro/dugble/server/internal/platform/systemmail"
)

func TestBuildVerificationEmailIsNeutral(t *testing.T) {
	renderer, err := systemmail.NewArgusRenderer()
	if err != nil {
		t.Fatalf("NewArgusRenderer() error = %v", err)
	}
	message, err := buildVerificationEmail(renderer, "482193", 10*time.Minute)
	if err != nil {
		t.Fatalf("buildVerificationEmail() error = %v", err)
	}
	combined := strings.ToLower(message.Subject + " " + message.Text + " " + message.HTML)
	if strings.Contains(combined, "dugble") {
		t.Fatalf("email content must not include Dugble branding: %q", combined)
	}
	if message.FromName != "Argus" {
		t.Fatalf("FromName = %q, want Argus", message.FromName)
	}
	for _, expected := range []string{"482193", "10 minutes", verificationEmailPreviewText} {
		if !strings.Contains(message.HTML, expected) {
			t.Fatalf("HTML does not contain %q", expected)
		}
	}
}

func TestBuildVerificationSMSIncludesDugble(t *testing.T) {
	message := buildVerificationSMS("482193", 10*time.Minute)
	for _, expected := range []string{"482193", "Dugble", "10 minutes", "Do not share it"} {
		if !strings.Contains(message, expected) {
			t.Fatalf("SMS does not contain %q: %q", expected, message)
		}
	}
}

func TestFormatVerificationExpiryRoundsUp(t *testing.T) {
	cases := []struct {
		remaining time.Duration
		want      string
	}{
		{remaining: 10 * time.Second, want: "1 minute"},
		{remaining: time.Minute, want: "1 minute"},
		{remaining: time.Minute + time.Second, want: "2 minutes"},
		{remaining: 10 * time.Minute, want: "10 minutes"},
	}
	for _, test := range cases {
		if got := formatVerificationExpiry(test.remaining); got != test.want {
			t.Errorf("formatVerificationExpiry(%s) = %q, want %q", test.remaining, got, test.want)
		}
	}
}
