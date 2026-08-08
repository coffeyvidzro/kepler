package feedback

import (
	"errors"
	"math"
	"time"
)

// RetryPolicy controls provider-status reconciliation after uncertain delivery.
type RetryPolicy struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
	MaxAttempts  int32
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		InitialDelay: 30 * time.Second,
		MaxDelay:     15 * time.Minute,
		Multiplier:   2,
		MaxAttempts:  8,
	}
}

func (policy RetryPolicy) Validate() error {
	if policy.InitialDelay <= 0 {
		return errors.New("feedback retry initial delay must be positive")
	}
	if policy.MaxDelay < policy.InitialDelay {
		return errors.New("feedback retry maximum delay cannot be shorter than the initial delay")
	}
	if policy.Multiplier < 1 {
		return errors.New("feedback retry multiplier must be at least one")
	}
	if policy.MaxAttempts < 0 {
		return errors.New("feedback retry maximum attempts cannot be negative")
	}
	return nil
}

// Delay returns the backoff for a one-based reconciliation attempt.
func (policy RetryPolicy) Delay(attempt int32) (time.Duration, bool) {
	if policy.Validate() != nil || attempt <= 0 {
		return 0, false
	}
	if policy.MaxAttempts > 0 && attempt > policy.MaxAttempts {
		return 0, false
	}

	delay := float64(policy.InitialDelay) * math.Pow(policy.Multiplier, float64(attempt-1))
	if delay >= float64(policy.MaxDelay) {
		return policy.MaxDelay, true
	}
	return time.Duration(delay), true
}

// Next returns the next reconciliation time for a one-based attempt.
func (policy RetryPolicy) Next(attempt int32, from time.Time) (time.Time, bool) {
	delay, ok := policy.Delay(attempt)
	if !ok || from.IsZero() {
		return time.Time{}, false
	}
	return from.Add(delay), true
}
