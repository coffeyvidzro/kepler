package billing

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Product string

type Channel string
type Settlement string

const (
	ProductSMS   Product = "sms"
	ProductEmail Product = "email"
)

const (
	ChannelSMS   Channel = "sms"
	ChannelEmail Channel = "email"
)

const SettlementAcceptedForDelivery Settlement = "accepted_for_delivery"

type SMSChargeInput struct {
	TeamID             uuid.UUID
	MessageID          uuid.UUID
	DestinationNumber  string
	Segments           int32
	destinationCountry string
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
	TeamID         uuid.UUID
	MessageID      uuid.UUID
	RecipientCount int64
}

// CommittedCharge is emitted only after the transaction containing the
// message, immediate billing mutation, and delivery outbox event has committed.
// The committed transaction is the billable acceptance boundary: later
// provider rejection, sender deactivation, cancellation, or delivery failure
// does not implicitly reverse wallet or allowance usage. Any future credit
// must be an explicit, separately audited billing operation.
type CommittedCharge struct {
	Charge
	Channel    Channel
	Settlement Settlement
	TeamID     uuid.UUID
	MessageID  uuid.UUID
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
