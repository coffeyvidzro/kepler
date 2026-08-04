package systemmail

import (
	"context"
	"strings"
	"testing"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"
)

type recordingSender struct{ messages []platformemail.Message }

func (s *recordingSender) Send(_ context.Context, message platformemail.Message) (platformemail.Result, error) {
	s.messages = append(s.messages, message)
	return platformemail.Result{}, nil
}

func TestSecurityNotificationsRenderAndSend(t *testing.T) {
	renderer, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	sender := &recordingSender{}
	service := NewEmailService(sender, renderer, "https://app.example.com", "security@example.com")
	tests := []struct {
		name string
		send func() error
		want string
	}{
		{name: "password changed", send: func() error {
			return service.SendPasswordChanged(context.Background(), SendPasswordChangedInput{ToEmail: "person@example.com", Name: "Person"})
		}, want: "password was changed"},
		{name: "email changed", send: func() error {
			return service.SendEmailChanged(context.Background(), SendEmailChangedInput{ToEmail: "person@example.com", Name: "Person", Email: "new@example.com"})
		}, want: "new@example.com"},
		{name: "MFA enabled", send: func() error {
			return service.SendMFAEnabled(context.Background(), SendSecurityEventInput{ToEmail: "person@example.com", Name: "Person"})
		}, want: "multi-factor authentication was enabled"},
		{name: "MFA disabled", send: func() error {
			return service.SendMFADisabled(context.Background(), SendSecurityEventInput{ToEmail: "person@example.com", Name: "Person"})
		}, want: "multi-factor authentication was disabled"},
		{name: "recovery code", send: func() error {
			return service.SendRecoveryCodeUsed(context.Background(), SendSecurityEventInput{ToEmail: "person@example.com", Name: "Person"})
		}, want: "recovery code was used"},
		{name: "account deleted", send: func() error {
			return service.SendAccountDeleted(context.Background(), SendSecurityEventInput{ToEmail: "person@example.com", Name: "Person"})
		}, want: "account was permanently deleted"},
		{name: "new login", send: func() error {
			return service.SendNewLogin(context.Background(), SendNewLoginInput{ToEmail: "person@example.com", Name: "Person", IPAddress: "203.0.113.10", UserAgent: "Example Browser", Method: "password"})
		}, want: "203.0.113.10"},
		{name: "failed MFA login", send: func() error {
			return service.SendMFALoginFailed(context.Background(), SendSecurityEventInput{ToEmail: "person@example.com", Name: "Person"})
		}, want: "failed the multi-factor authentication step"},
		{name: "member removed", send: func() error {
			return service.SendTeamMemberRemoved(context.Background(), SendTeamMemberChangedInput{ToEmail: "person@example.com", Name: "Person", Team: "Example"})
		}, want: "Example</strong> team"},
		{name: "member role changed", send: func() error {
			return service.SendTeamMemberRoleChanged(context.Background(), SendTeamMemberChangedInput{ToEmail: "person@example.com", Name: "Person", Team: "Example", Role: "admin"})
		}, want: "admin</strong>"},
		{name: "team token created", send: func() error {
			return service.SendTeamTokenCreated(context.Background(), SendTeamTokenChangedInput{ToEmail: "person@example.com", Name: "Person", TeamID: "team-id", TokenName: "CI", TokenPrefix: "dgb_team_abcd"})
		}, want: "CI</strong>"},
		{name: "team token revoked", send: func() error {
			return service.SendTeamTokenRevoked(context.Background(), SendTeamTokenChangedInput{ToEmail: "person@example.com", Name: "Person", TeamID: "team-id", TokenName: "CI", TokenPrefix: "dgb_team_abcd"})
		}, want: "can no longer authenticate"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := len(sender.messages)
			if err := test.send(); err != nil {
				t.Fatal(err)
			}
			if len(sender.messages) != before+1 {
				t.Fatalf("sent messages = %d, want %d", len(sender.messages), before+1)
			}
			message := sender.messages[len(sender.messages)-1]
			if message.To[0].Email != "person@example.com" || !strings.Contains(message.HTML, test.want) {
				t.Fatalf("unexpected message: %#v", message)
			}
		})
	}
}
