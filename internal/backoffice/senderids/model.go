package senderids

import "time"

type Filter struct {
	Query  string
	Status string
}

type Row struct {
	ID          string
	TeamName    string
	Name        string
	CountryCode string
	Status      string
	CreatedAt   time.Time
}

type Detail struct {
	ID              string
	TeamID          string
	TeamName        string
	Name            string
	CountryCode     string
	Purpose         string
	Status          string
	Provider        string
	RejectionReason string
	ApprovedAt      string
	RejectedAt      string
	SuspendedAt     string
	CreatedBy       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type StatusRequest struct {
	Action string
	Reason string
}
