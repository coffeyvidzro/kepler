package sms

import (
	"errors"

	"github.com/google/uuid"
)

var ErrNoEligibleRoute = errors.New("no eligible SMS delivery route")

// DeliveryRoute is a provider route for one approved Sender ID.
type DeliveryRoute struct {
	SenderID uuid.UUID
	Provider string
}
