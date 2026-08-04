package domains

import "time"

type Filter struct {
	Query  string
	Status string
}

type Row struct {
	ID        string
	TeamName  string
	Domain    string
	Provider  string
	Status    string
	CreatedAt time.Time
}

type Detail struct {
	ID                  string
	TeamID              string
	TeamName            string
	Domain              string
	Provider            string
	ProviderRegion      string
	Status              string
	VerificationRecords string
	FailureReason       string
	LastCheckedAt       string
	VerifiedAt          string
	DisabledAt          string
	CreatedBy           string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type StatusRequest struct {
	Action string
	Reason string
}
