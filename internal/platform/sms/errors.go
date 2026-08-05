package sms

import "errors"

var (
	ErrRouterRequired        = errors.New("SMS router is required")
	ErrNoProviderAvailable   = errors.New("no SMS provider is available")
	ErrProviderNotFound      = errors.New("SMS provider not found")
	ErrInvalidProviderReply  = errors.New("invalid SMS provider response")
	ErrProviderRequired      = errors.New("SMS provider is required")
	ErrInvalidProviderID     = errors.New("invalid SMS provider ID")
	ErrDuplicateProvider     = errors.New("duplicate SMS provider")
	ErrProviderNotRegistered = errors.New("SMS provider is not registered")
)
