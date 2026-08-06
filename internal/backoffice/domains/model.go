package domains

import "time"

// Domain is the administrative view of an email sender domain and its active
// provider binding.
type Domain struct {
	ID                        string     `json:"id"`
	AssetID                   string     `json:"asset_id"`
	TeamID                    string     `json:"team_id,omitempty"`
	TeamName                  string     `json:"team_name,omitempty"`
	Name                      string     `json:"name"`
	OwnerType                 string     `json:"owner_type"`
	AssetStatus               string     `json:"asset_status"`
	Provider                  string     `json:"provider,omitempty"`
	ProviderAccount           string     `json:"provider_account"`
	Region                    string     `json:"region,omitempty"`
	Status                    string     `json:"status"`
	ProviderStatus            string     `json:"provider_status,omitempty"`
	Verified                  bool       `json:"verified"`
	HealthStatus              string     `json:"health_status"`
	Attempts                  int32      `json:"attempts"`
	ConsecutiveHealthFailures int32      `json:"consecutive_health_failures"`
	LastError                 string     `json:"last_error,omitempty"`
	LastCheckedAt             *time.Time `json:"last_checked_at,omitempty"`
	NextCheckAt               time.Time  `json:"next_check_at"`
	CreatedAt                 time.Time  `json:"created_at"`
	UpdatedAt                 time.Time  `json:"updated_at"`
}

type ListInput struct {
	Limit  int32
	Offset int32
}

type Page struct {
	Domains []Domain `json:"domains"`
	Limit   int32    `json:"limit"`
	Offset  int32    `json:"offset"`
}
