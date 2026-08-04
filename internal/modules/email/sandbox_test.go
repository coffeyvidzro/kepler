package email

import (
	"context"
	"testing"

	"github.com/google/uuid"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"
)

type sandboxAuthorizerStub struct {
	err    error
	called bool
	to     []EmailAddress
	cc     []EmailAddress
	bcc    []EmailAddress
}

func (s *sandboxAuthorizerStub) AuthorizeSandboxRecipients(
	_ context.Context,
	_ uuid.UUID,
	to, cc, bcc []EmailAddress,
) error {
	s.called = true
	s.to, s.cc, s.bcc = to, cc, bcc
	return s.err
}

func TestAuthorizeSenderRejectsUnverifiedSandboxTeamEmail(t *testing.T) {
	service := &Service{
		config:            ServiceConfig{DefaultProvider: "aws_ses", DefaultRegion: "eu-north-1"},
		sandboxRecipients: &sandboxAuthorizerStub{err: ErrSandboxTeamEmailNotVerified},
	}
	message := validatedSend{
		MessageType: MessageTypeTransactional,
		FromEmail:   platformemail.CustomerOnboardingIdentity,
		To:          []EmailAddress{{Email: "owner@example.com"}},
	}

	if err := service.authorizeSender(context.Background(), uuid.New(), &message); err == nil {
		t.Fatal("expected unverified team email to be rejected")
	}
}

func TestAuthorizeSenderRejectsRestrictedSandboxRecipient(t *testing.T) {
	service := &Service{
		config:            ServiceConfig{DefaultProvider: "aws_ses", DefaultRegion: "eu-north-1"},
		sandboxRecipients: &sandboxAuthorizerStub{err: ErrSandboxRecipientRestricted},
	}
	message := validatedSend{
		MessageType: MessageTypeTransactional,
		FromEmail:   platformemail.CustomerOnboardingIdentity,
		To:          []EmailAddress{{Email: "other@example.com"}},
	}

	if err := service.authorizeSender(context.Background(), uuid.New(), &message); err == nil {
		t.Fatal("expected restricted sandbox recipient to be rejected")
	}
}
