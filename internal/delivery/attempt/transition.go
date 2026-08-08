package attempt

// CanTransitionTo defines monotonic provider-attempt lifecycle transitions.
// A retryable failure starts a new attempt rather than reopening this attempt.
func (status AttemptStatus) CanTransitionTo(next AttemptStatus) bool {
	if !status.Valid() || !next.Valid() {
		return false
	}
	if status == next {
		return true
	}
	if status.Terminal() || status == StatusRetryableFailure {
		return false
	}

	switch status {
	case StatusClaimed:
		return next == StatusRequestStarted || next == StatusRetryableFailure ||
			next == StatusPermanentFailure || next == StatusCanceled
	case StatusRequestStarted:
		// A provider callback can prove submission even when the worker lost
		// the synchronous response before it persisted submission_unknown.
		return next == StatusSubmissionUnknown || next == StatusSubmitted ||
			next == StatusAccepted || next == StatusSent || next == StatusDelivered ||
			next == StatusRetryableFailure || next == StatusPermanentFailure ||
			next == StatusRejected || next == StatusExpired || next == StatusUnknown ||
			next == StatusCanceled
	case StatusSubmissionUnknown:
		return next == StatusSubmitted || next == StatusAccepted || next == StatusSent ||
			next == StatusDelivered || next == StatusRetryableFailure ||
			next == StatusPermanentFailure || next == StatusRejected ||
			next == StatusExpired || next == StatusUnknown
	case StatusSubmitted:
		return next == StatusAccepted || next == StatusSent || next == StatusDelivered ||
			next == StatusRetryableFailure || next == StatusPermanentFailure ||
			next == StatusRejected || next == StatusExpired || next == StatusUnknown
	case StatusAccepted:
		return next == StatusSent || next == StatusDelivered ||
			next == StatusRetryableFailure || next == StatusPermanentFailure ||
			next == StatusRejected || next == StatusExpired || next == StatusUnknown
	case StatusSent:
		return next == StatusDelivered || next == StatusPermanentFailure ||
			next == StatusRejected || next == StatusExpired || next == StatusUnknown
	case StatusUnknown:
		return next == StatusSubmitted || next == StatusAccepted || next == StatusSent ||
			next == StatusDelivered || next == StatusPermanentFailure ||
			next == StatusRejected || next == StatusExpired
	default:
		return false
	}
}
