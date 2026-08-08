package messagingrouting

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging"
	platformrouting "github.com/coffeyvidzro/dugble/server/internal/platform/messaging/routing"
)

type recordingSource struct {
	called bool
}

func (source *recordingSource) ListCandidates(_ context.Context, _ platformrouting.Request) ([]platformrouting.Candidate, error) {
	source.called = true
	return []platformrouting.Candidate{}, nil
}

func TestRepositoryListCandidatesUsesChannelDataSource(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		channel messaging.Channel
	}{
		{name: "email", channel: messaging.ChannelEmail},
		{name: "SMS", channel: messaging.ChannelSMS},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			email := &recordingSource{}
			sms := &recordingSource{}
			repository := &Repository{email: email, sms: sms}

			if _, err := repository.ListCandidates(context.Background(), platformrouting.Request{
				TeamID:  uuid.New(),
				Channel: test.channel,
			}); err != nil {
				t.Fatalf("ListCandidates() error = %v", err)
			}
			if email.called != (test.channel == messaging.ChannelEmail) {
				t.Fatalf("email source called = %t", email.called)
			}
			if sms.called != (test.channel == messaging.ChannelSMS) {
				t.Fatalf("SMS source called = %t", sms.called)
			}
		})
	}
}

func TestRepositoryListCandidatesRejectsUnsupportedChannel(t *testing.T) {
	t.Parallel()
	repository := &Repository{email: &recordingSource{}, sms: &recordingSource{}}
	if _, err := repository.ListCandidates(context.Background(), platformrouting.Request{Channel: "push"}); err == nil {
		t.Fatal("ListCandidates() error = nil, want unsupported channel error")
	}
}
