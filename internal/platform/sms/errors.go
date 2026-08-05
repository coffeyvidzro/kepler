package sms

import "errors"

var (
	ErrRouterRequired        = errors.New("SMS router is required")
	ErrNoProviderAvailable   = errors.New("no SMS provider is available")
	ErrProviderNotFound      = errors.New("SMS provider not found")
	ErrInvalidProviderReply  = errors.New("invalid SMS provider response")
	ErrNoRoutesConfigured    = errors.New("no SMS routes configured")
	ErrNoEnabledRoutes       = errors.New("no SMS routes are enabled")
	ErrInvalidProviderID     = errors.New("invalid SMS provider ID")
	ErrInvalidCountryCode    = errors.New("invalid SMS destination country")
	ErrDuplicateProvider     = errors.New("duplicate SMS provider")
	ErrDuplicateCountry      = errors.New("duplicate SMS destination country")
	ErrRoutingServiceNil     = errors.New("SMS routing service is nil")
	ErrProviderRequired      = errors.New("SMS provider is required")
	ErrProviderNotRegistered = errors.New("SMS provider is not registered")
)
