package users

import "time"

type Filter struct {
	Query string
}

type Row struct {
	ID            string
	Email         string
	Name          string
	EmailVerified bool
	CreatedAt     time.Time
}

type Detail struct {
	User  Row
	Teams []TeamMembershipRow
}

type TeamMembershipRow struct {
	ID     string
	Name   string
	Role   string
	Status string
}
