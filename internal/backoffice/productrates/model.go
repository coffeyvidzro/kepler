package productrates

import "time"

type ProductRate struct {
	ID             string     `json:"id"`
	Product        string     `json:"product"`
	Meter          string     `json:"meter"`
	BillingMarket  string     `json:"billing_market"`
	Tier           string     `json:"tier"`
	Currency       string     `json:"currency"`
	CostUnits      int64      `json:"cost_units"`
	EffectiveFrom  time.Time  `json:"effective_from"`
	EffectiveUntil *time.Time `json:"effective_until,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type ListInput struct {
	Limit  int32
	Offset int32
}

type Page struct {
	ProductRates []ProductRate `json:"product_rates"`
	Limit        int32         `json:"limit"`
	Offset       int32         `json:"offset"`
}

type CreateInput struct {
	Product        string     `json:"product"`
	Meter          string     `json:"meter"`
	BillingMarket  string     `json:"billing_market"`
	Tier           string     `json:"tier"`
	Currency       string     `json:"currency"`
	CostUnits      int64      `json:"cost_units"`
	EffectiveFrom  time.Time  `json:"effective_from"`
	EffectiveUntil *time.Time `json:"effective_until,omitempty"`
}

type CloseInput struct {
	EffectiveUntil time.Time `json:"effective_until"`
}
