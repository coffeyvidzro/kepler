package attempt

import (
	"fmt"
	"strings"
)

// Channel identifies a customer messaging channel.
type Channel string

const (
	ChannelEmail Channel = "email"
	ChannelSMS   Channel = "sms"
)

// ParseChannel normalizes and validates a messaging channel.
func ParseChannel(value string) (Channel, error) {
	channel := Channel(strings.ToLower(strings.TrimSpace(value)))
	if !channel.Valid() {
		return "", fmt.Errorf("unsupported messaging channel %q", value)
	}
	return channel, nil
}

// Valid reports whether the channel is supported by the messaging control plane.
func (channel Channel) Valid() bool {
	switch channel {
	case ChannelEmail, ChannelSMS:
		return true
	default:
		return false
	}
}
