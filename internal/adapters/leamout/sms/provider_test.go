package sms

import (
	"context"
	"testing"

	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

func TestProviderProgressesToDelivered(t *testing.T) {
	t.Parallel()

	provider := NewProvider()
	created, err := provider.Send(context.Background(), platformsms.SendRequest{
		Reference:          "message-1",
		To:                 "+2348000000000",
		From:               "Dugble",
		Message:            "Hello",
		DestinationCountry: platformsms.CountryNigeria,
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if created.ProviderID != ProviderID || created.Status != platformsms.StatusSubmitted || created.ProviderMsgID == "" {
		t.Fatalf("Send() = %#v", created)
	}

	first, err := provider.CheckStatus(context.Background(), created.ProviderMsgID)
	if err != nil {
		t.Fatalf("first CheckStatus() error = %v", err)
	}
	second, err := provider.CheckStatus(context.Background(), created.ProviderMsgID)
	if err != nil {
		t.Fatalf("second CheckStatus() error = %v", err)
	}
	if first.Status != platformsms.StatusSent || second.Status != platformsms.StatusDelivered {
		t.Fatalf("statuses = %q, %q", first.Status, second.Status)
	}
}

func TestProviderRejectsUnknownStatusConfiguration(t *testing.T) {
	t.Parallel()

	if _, err := NewProviderWithConfig(Config{StatusSequence: []string{"invented"}}); err == nil {
		t.Fatal("NewProviderWithConfig() error = nil")
	}
}
