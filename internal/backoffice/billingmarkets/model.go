package billingmarkets

type BillingMarket struct {
	Code       string `json:"code"`
	Currency   string `json:"currency"`
	MinorUnit  int16  `json:"minor_unit"`
	IsEnabled  bool   `json:"is_enabled"`
}

type ListInput struct {
	Limit  int32
	Offset int32
}

type Page struct {
	BillingMarkets []BillingMarket `json:"billing_markets"`
	Limit          int32           `json:"limit"`
	Offset         int32           `json:"offset"`
}

type CreateInput struct {
	Code      string `json:"code"`
	Currency  string `json:"currency"`
	IsEnabled *bool  `json:"is_enabled,omitempty"`
}

type UpdateInput struct {
	IsEnabled bool `json:"is_enabled"`
}
