package smsrates

import "time"

type SMSRate struct {
	ID                 string     `json:"id"`
	BillingMarket      string     `json:"billing_market"`
	DestinationCountry string     `json:"destination_country"`
	RouteType          string     `json:"route_type"`
	Tier               string     `json:"tier"`
	Currency           string     `json:"currency"`
	CostUnits          int64      `json:"cost_units"`
	EffectiveFrom      time.Time  `json:"effective_from"`
	EffectiveUntil     *time.Time `json:"effective_until,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
}

type ListInput struct {
	Limit  int32
	Offset int32
}

type Page struct {
	SMSRates []SMSRate `json:"sms_rates"`
	Limit    int32     `json:"limit"`
	Offset   int32     `json:"offset"`
}

type CreateInput struct {
	BillingMarket      string     `json:"billing_market"`
	DestinationCountry string     `json:"destination_country"`
	RouteType          string     `json:"route_type"`
	Tier               string     `json:"tier"`
	Currency           string     `json:"currency"`
	CostUnits          int64      `json:"cost_units"`
	EffectiveFrom      time.Time  `json:"effective_from"`
	EffectiveUntil     *time.Time `json:"effective_until,omitempty"`
}

type CloseInput struct {
	EffectiveUntil time.Time `json:"effective_until"`
}
