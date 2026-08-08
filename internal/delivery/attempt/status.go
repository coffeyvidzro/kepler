package attempt

// AttemptStatus is the channel-neutral lifecycle state of one provider attempt.
type AttemptStatus string

const (
	StatusClaimed           AttemptStatus = "claimed"
	StatusRequestStarted    AttemptStatus = "request_started"
	StatusSubmissionUnknown AttemptStatus = "submission_unknown"
	StatusSubmitted         AttemptStatus = "submitted"
	StatusAccepted          AttemptStatus = "accepted"
	StatusSent              AttemptStatus = "sent"
	StatusDelivered         AttemptStatus = "delivered"
	StatusRetryableFailure  AttemptStatus = "retryable_failure"
	StatusPermanentFailure  AttemptStatus = "permanent_failure"
	StatusRejected          AttemptStatus = "rejected"
	StatusExpired           AttemptStatus = "expired"
	StatusCanceled          AttemptStatus = "canceled"
	StatusUnknown           AttemptStatus = "unknown"
)

func (status AttemptStatus) Valid() bool {
	switch status {
	case StatusClaimed, StatusRequestStarted, StatusSubmissionUnknown,
		StatusSubmitted, StatusAccepted, StatusSent, StatusDelivered,
		StatusRetryableFailure, StatusPermanentFailure, StatusRejected,
		StatusExpired, StatusCanceled, StatusUnknown:
		return true
	default:
		return false
	}
}

func (status AttemptStatus) Terminal() bool {
	switch status {
	case StatusDelivered, StatusPermanentFailure, StatusRejected,
		StatusExpired, StatusCanceled:
		return true
	default:
		return false
	}
}

func (status AttemptStatus) NeedsReconciliation() bool {
	switch status {
	case StatusSubmissionUnknown, StatusSubmitted, StatusAccepted,
		StatusSent, StatusUnknown:
		return true
	default:
		return false
	}
}

func (status AttemptStatus) RequiresProvider() bool {
	switch status {
	case StatusSubmitted, StatusAccepted, StatusSent, StatusDelivered,
		StatusRejected, StatusExpired:
		return true
	default:
		return false
	}
}
