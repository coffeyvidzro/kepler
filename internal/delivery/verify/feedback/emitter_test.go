package feedback

import (
	"testing"

	platformwebhook "github.com/coffeyvidzro/dugble/server/internal/platform/webhook"
)

func TestTerminalFailureChannel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		eventType string
		channel   string
		terminal  bool
	}{
		{eventType: platformwebhook.EventEmailBounced, channel: "email", terminal: true},
		{eventType: platformwebhook.EventEmailRejected, channel: "email", terminal: true},
		{eventType: platformwebhook.EventEmailFailed, channel: "email", terminal: true},
		{eventType: platformwebhook.EventEmailComplained},
		{eventType: platformwebhook.EventEmailDelivered},
		{eventType: platformwebhook.EventSMSUndelivered, channel: "sms", terminal: true},
		{eventType: platformwebhook.EventSMSFailed, channel: "sms", terminal: true},
		{eventType: platformwebhook.EventSMSDelivered},
		{eventType: platformwebhook.EventSMSSent},
	}

	for _, test := range tests {
		test := test
		t.Run(test.eventType, func(t *testing.T) {
			t.Parallel()
			channel, terminal := terminalFailureChannel(test.eventType)
			if channel != test.channel || terminal != test.terminal {
				t.Fatalf("terminalFailureChannel(%q) = (%q, %v), want (%q, %v)", test.eventType, channel, terminal, test.channel, test.terminal)
			}
		})
	}
}

func TestChannelMessageColumn(t *testing.T) {
	t.Parallel()

	column, err := channelMessageColumn("email")
	if err != nil || column != "email_message_id" {
		t.Fatalf("channelMessageColumn(email) = (%q, %v)", column, err)
	}
	column, err = channelMessageColumn("sms")
	if err != nil || column != "sms_message_id" {
		t.Fatalf("channelMessageColumn(sms) = (%q, %v)", column, err)
	}
	if _, err := channelMessageColumn("push"); err == nil {
		t.Fatal("channelMessageColumn(push) returned nil error")
	}
}
