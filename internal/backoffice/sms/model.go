package sms

import "time"

type Filter struct {
	Query  string
	Status string
}

type Row struct {
	ID           string
	TeamName     string
	ToNumber     string
	FromName     string
	Status       string
	ProviderID   string
	ErrorMessage string
	CreatedAt    time.Time
}

type Detail struct {
	ID                string
	TeamID            string
	TeamName          string
	SenderID          string
	ToNumber          string
	FromName          string
	Body              string
	Status            string
	ProviderID        string
	ProviderMessageID string
	Segments          int32
	ErrorMessage      string
	Metadata          string
	SubmittedAt       string
	DeliveredAt       string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
