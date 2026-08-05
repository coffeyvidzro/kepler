package sms

import (
	"context"
	"errors"
	"testing"

	"github.com/coffeyvidzro/dugble/server/internal/adapters/runnage"
	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

func validRequest() platformsms.SendRequest {
	return platformsms.SendRequest{
		Reference:          "message-1",
		To:                 "+2348000000001",
		From:               "Dugble",
		Message:            "Hello",
		DestinationCountry: platformsms.CountryNigeria,
	}
}

func TestProviderProgressesToUndelivered(t *testing.T) {
	t.Parallel()

	provider := NewProvider()
	created, err := provider.Send(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	first, err := provider.CheckStatus(context.Background(), created.ProviderMsgID)
	if err != nil {
		t.Fatalf("first CheckStatus() error = %v", err)
	}
	second, err := provider.CheckStatus(context.Background(), created.ProviderMsgID)
	if err != nil {
		t.Fatalf("second CheckStatus() error = %v", err)
	}
	if first.Status != platformsms.StatusSent || second.Status != platformsms.StatusUndelivered {
		t.Fatalf("statuses = %q, %q", first.Status, second.Status)
	}
}

func TestProviderExposesDefinitiveAndUncertainErrors(t *testing.T) {
	t.Parallel()

	rejected, err := NewProviderWithConfig(Config{SendMode: SendModeRejected})
	if err != nil {
		t.Fatalf("NewProviderWithConfig(rejected) error = %v", err)
	}
	_, rejectedErr := rejected.Send(context.Background(), validRequest())

	uncertain, err := NewProviderWithConfig(Config{SendMode: SendModeUncertain})
	if err != nil {
		t.Fatalf("NewProviderWithConfig(uncertain) error = %v", err)
	}
	_, uncertainErr := uncertain.Send(context.Background(), validRequest())

	var rejectedProviderErr *runnage.Error
	if !errors.As(rejectedErr, &rejectedProviderErr) || !rejectedProviderErr.SafeToFallback() {
		t.Fatalf("rejected error = %#v", rejectedErr)
	}
	var uncertainProviderErr *runnage.Error
	if !errors.As(uncertainErr, &uncertainProviderErr) || uncertainProviderErr.SafeToFallback() {
		t.Fatalf("uncertain error = %#v", uncertainErr)
	}
}
