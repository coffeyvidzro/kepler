package attempt

import (
	"errors"

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
