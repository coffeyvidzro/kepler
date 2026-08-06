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

type SMSChargeInput struct {
	TeamID             uuid.UUID
	MessageID          uuid.UUID
	DestinationNumber  string
	Segments           int32
	destinationCountry string
	provider           string
	routeType          string
}

type Charge struct {
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

type EmailChargeInput struct {
	TeamID        uuid.UUID
	MessageID     uuid.UUID
	RecipientCount int64
}

// CommittedCharge is emitted only after the transaction containing the
// message, immediate billing mutation, and delivery outbox event has committed.
type CommittedCharge struct {
	Charge
	Channel   Channel
	TeamID    uuid.UUID
	MessageID uuid.UUID
}

type ChargeObserver interface {
	ObserveCommittedCharge(context.Context, CommittedCharge)
}

type SMSCharger interface {
	ChargeSMS(context.Context, pgx.Tx, SMSChargeInput) (Charge, error)
}

type EmailCharger interface {
	ChargeEmail(context.Context, pgx.Tx, EmailChargeInput) (Charge, error)
}

type SMSBilling interface {
	SMSCharger
	ChargeObserver
}

type EmailBilling interface {
	EmailCharger
	ChargeObserver
}
