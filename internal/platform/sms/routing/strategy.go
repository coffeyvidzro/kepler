package routing

import (
	"context"
	"errors"
	"sort"
)

type Strategy interface {
	Order(context.Context, string, []Route) []Route
	ShouldFallback(context.Context, string, error) bool
}

// SafeFallbackError is implemented only when a provider definitely rejected
// the request before accepting the SMS.
type SafeFallbackError interface {
	error
	SafeToFallback() bool
}

type PriorityStrategy struct{}

func NewPriorityStrategy() *PriorityStrategy {
	return &PriorityStrategy{}
}

func (*PriorityStrategy) Order(
	_ context.Context,
	_ string,
	routes []Route,
) []Route {
	ordered := make([]Route, len(routes))
	copy(ordered, routes)
	sort.SliceStable(ordered, func(left, right int) bool {
		return ordered[left].Priority < ordered[right].Priority
	})
	return ordered
}

func (*PriorityStrategy) ShouldFallback(
	_ context.Context,
	_ string,
	err error,
) bool {
	if err == nil {
		return false
	}

	var fallbackError SafeFallbackError
	if !errors.As(err, &fallbackError) {
		// Timeouts, connection resets, decode failures, and unknown errors may
		// occur after the provider accepted the SMS.
		return false
	}
	return fallbackError.SafeToFallback()
}
