package security

import "time"

// Outcome describes the result of a security evaluation.
type Outcome string

const (
	OutcomeAllow    Outcome = "allow"
	OutcomeDeny     Outcome = "deny"
	OutcomeThrottle Outcome = "throttle"
	OutcomeObserve  Outcome = "observe"
)

// Reason explains why a security decision was made.
type Reason struct {
	Code    string
	Message string
}

// Decision is the aggregate result returned by the security runtime.
type Decision struct {
	Outcome    Outcome
	Reasons    []Reason
	RetryAfter time.Duration
}

// Allow returns an allow decision.
func Allow() Decision {
	return Decision{Outcome: OutcomeAllow}
}

// Allowed reports whether request processing may continue.
func (d Decision) Allowed() bool {
	return d.Outcome == OutcomeAllow || d.Outcome == OutcomeObserve
}
