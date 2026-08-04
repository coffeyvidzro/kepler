package billing

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Product string

type Channel string

const (
	ProductSMS   Product = "sms"
	ProductEmail Product = "email"
)

const (
	ChannelSMS   Channel = "sms"
	ChannelEmail Channel = "email"
)

type SMSAuthorizationInput struct {
	TeamID             uuid.UUID
	MessageID          uuid.UUID
	DestinationNumber  string
	Segments           int32
	destinationCountry string
	provider           string
	routeType          string
}

type Authorization struct {
	Outcome            Outcome
	MarketCode         string
	Currency           string
	Tier               string
	Product            Product
	UnitCostUnits      int64
	Quantity           int64
	AmountUnits        int64
	RemainingBalance   int64
	CoveredByAllowance bool
	RemainingAllowance int64
}

type EmailAuthorizationInput struct {
	TeamID    uuid.UUID
	MessageID uuid.UUID
}

// CommittedAuthorization is emitted only after the transaction containing the
// message, billing mutation, and delivery outbox event has committed.
type CommittedAuthorization struct {
	Authorization
	Channel   Channel
	TeamID    uuid.UUID
	MessageID uuid.UUID
}

type CommitObserver interface {
	ObserveCommitted(context.Context, CommittedAuthorization)
}

type SMSAuthorizer interface {
	AuthorizeSMS(context.Context, pgx.Tx, SMSAuthorizationInput) (Authorization, error)
}

type EmailAuthorizer interface {
	AuthorizeEmail(context.Context, pgx.Tx, EmailAuthorizationInput) (Authorization, error)
}

type SMSBilling interface {
	SMSAuthorizer
	CommitObserver
}

type EmailBilling interface {
	EmailAuthorizer
	CommitObserver
}
