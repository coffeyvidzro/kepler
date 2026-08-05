package senderid

import "context"

const (
	ProviderLeamout = "leamout"
	ProviderMNotify = "mnotify"
	ProviderMoolre  = "moolre"
	ProviderRunnage = "runnage"

	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusRejected = "rejected"
	StatusUnknown  = "unknown"
)

type CreateRequest struct {
	SenderID string
	Purpose  string
}

type CreateResponse struct {
	ProviderID string
	SenderID   string
	Status     string
}

type StatusResponse struct {
	ProviderID     string
	SenderID       string
	Status         string
	ProviderStatus string
	Whitelisted    bool
}

type Provider interface {
	ID() string
	Create(context.Context, CreateRequest) (*CreateResponse, error)
	CheckStatus(context.Context, string) (*StatusResponse, error)
}
