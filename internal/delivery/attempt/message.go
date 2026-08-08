package attempt

import (
	"errors"
	"strings"

	"github.com/google/uuid"
)

// MessageReference identifies exactly one channel-specific message.
type MessageReference struct {
	Channel        Channel
	EmailMessageID *uuid.UUID
	SMSMessageID   *uuid.UUID
}

func (reference MessageReference) Validate() error {
	if !reference.Channel.Valid() {
		return errors.New("delivery message channel is invalid")
	}
	emailSet := reference.EmailMessageID != nil && *reference.EmailMessageID != uuid.Nil
	smsSet := reference.SMSMessageID != nil && *reference.SMSMessageID != uuid.Nil
	if emailSet == smsSet {
		return errors.New("delivery attempt must reference exactly one message")
	}
	if reference.Channel == ChannelEmail && !emailSet {
		return errors.New("email delivery attempt requires an email message")
	}
	if reference.Channel == ChannelSMS && !smsSet {
		return errors.New("SMS delivery attempt requires an SMS message")
	}
	return nil
}

// ID returns the underlying channel-specific message identifier.
func (reference MessageReference) ID() (uuid.UUID, bool) {
	if err := reference.Validate(); err != nil {
		return uuid.Nil, false
	}
	if reference.Channel == ChannelEmail {
		return *reference.EmailMessageID, true
	}
	return *reference.SMSMessageID, true
}

// ProviderRoute is the immutable provider and sender snapshot used for an attempt.
type ProviderRoute struct {
	Provider        string
	ProviderAccount string
	SenderDomainID  *uuid.UUID
	SenderID        *uuid.UUID
}

func (route ProviderRoute) Validate(requireProvider bool) error {
	if strings.TrimSpace(route.ProviderAccount) == "" {
		return errors.New("delivery attempt provider account is required")
	}
	if requireProvider && strings.TrimSpace(route.Provider) == "" {
		return errors.New("delivery attempt provider is required for the current status")
	}
	if route.SenderDomainID != nil && *route.SenderDomainID == uuid.Nil {
		return errors.New("delivery attempt sender domain ID is invalid")
	}
	if route.SenderID != nil && *route.SenderID == uuid.Nil {
		return errors.New("delivery attempt Sender ID is invalid")
	}
	if route.SenderDomainID != nil && route.SenderID != nil {
		return errors.New("delivery attempt cannot reference both a sender domain and Sender ID")
	}
	return nil
}
