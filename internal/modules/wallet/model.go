package wallet

import "time"

type Wallet struct {
	TeamID        string    `json:"team_id"`
	BillingMarket string    `json:"billing_market"`
	Currency      string    `json:"currency"`
	BalanceUnits  int64     `json:"balance_units"`
	Tier          string    `json:"tier"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type LedgerEntry struct {
	ID                   string    `json:"id"`
	TeamID               string    `json:"team_id"`
	UsageAuthorizationID *string   `json:"usage_authorization_id,omitempty"`
	AmountUnits          int64     `json:"amount_units"`
	TransactionType      string    `json:"transaction_type"`
	ReferenceID          string    `json:"reference_id"`
	CreatedAt            time.Time `json:"created_at"`
}

type LedgerPage struct {
	Entries []LedgerEntry `json:"entries"`
	Limit   int32         `json:"limit"`
	Offset  int32         `json:"offset"`
}

type CreditInput struct {
	TeamID      string
	AmountUnits int64
	ReferenceID string
}
