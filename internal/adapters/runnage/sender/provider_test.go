package sender

import (
	"context"
	"errors"
	"testing"

	"github.com/coffeyvidzro/dugble/server/internal/adapters/runnage"
	platformsenderid "github.com/coffeyvidzro/dugble/server/internal/platform/senderid"
)

func validRequest() platformsenderid.CreateRequest {
	return platformsenderid.CreateRequest{
		SenderID: "DugbleNG",
		Purpose:  "Transactional messages",
	}
}

func TestProviderProgressesToRejected(t *testing.T) {
	t.Parallel()

	provider := NewProvider()
	created, err := provider.Create(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Status != platformsenderid.StatusPending {
		t.Fatalf("Create() status = %q", created.Status)
	}
	status, err := provider.CheckStatus(context.Background(), created.SenderID)
	if err != nil {
		t.Fatalf("CheckStatus() error = %v", err)
	}
	if status.Status != platformsenderid.StatusRejected {
		t.Fatalf("CheckStatus() = %#v", status)
	}
}

func TestProviderSupportsImmediateRejectionAndUncertainSubmission(t *testing.T) {
	t.Parallel()

	rejected, err := NewProviderWithConfig(Config{CreateMode: CreateModeRejected})
	if err != nil {
		t.Fatalf("NewProviderWithConfig(rejected) error = %v", err)
	}
	created, err := rejected.Create(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("rejected Create() error = %v", err)
	}
	if created.Status != platformsenderid.StatusRejected {
		t.Fatalf("rejected Create() = %#v", created)
	}

	uncertain, err := NewProviderWithConfig(Config{CreateMode: CreateModeUncertain})
	if err != nil {
		t.Fatalf("NewProviderWithConfig(uncertain) error = %v", err)
	}
	_, uncertainErr := uncertain.Create(context.Background(), validRequest())
	var providerErr *runnage.Error
	if !errors.As(uncertainErr, &providerErr) || providerErr.SafeToFallback() {
		t.Fatalf("uncertain error = %#v", uncertainErr)
	}
}
