package sender

import (
	"context"
	"testing"

	platformsenderid "github.com/coffeyvidzro/dugble/server/internal/platform/senderid"
)

func TestProviderApprovesSenderID(t *testing.T) {
	t.Parallel()

	provider := NewProvider()
	created, err := provider.Create(context.Background(), platformsenderid.CreateRequest{
		SenderID: "DugbleKE",
		Purpose:  "Transactional messages",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ProviderID != ProviderID || created.Status != platformsenderid.StatusPending {
		t.Fatalf("Create() = %#v", created)
	}

	status, err := provider.CheckStatus(context.Background(), "dugbleke")
	if err != nil {
		t.Fatalf("CheckStatus() error = %v", err)
	}
	if status.Status != platformsenderid.StatusApproved || !status.Whitelisted {
		t.Fatalf("CheckStatus() = %#v", status)
	}
}
