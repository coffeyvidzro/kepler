package feedback

import "time"

func aggregateTransitionFromCounts(
	counts map[string]int,
	total int,
	fallbackStatus string,
	latestDeliveredAt *time.Time,
	latestFailedAt *time.Time,
) aggregateTransition {
	if total == 0 {
		return aggregateTransition{status: fallbackStatus}
	}

	delivered := counts[recipientStatusDelivered]
	complained := counts[recipientStatusComplained]
	bounced := counts[recipientStatusBounced]
	rejected := counts[recipientStatusRejected]
	failed := counts[recipientStatusFailed]
	terminalFailures := complained + bounced + rejected + failed

	transition := aggregateTransition{deliveredAt: latestDeliveredAt, failedAt: latestFailedAt}
	switch {
	case complained > 0:
		transition.status = "complained"
		transition.errorCode = stringPointer("ses_complaint")
		transition.errorMessage = stringPointer("SES reported a complaint for at least one recipient")
	case delivered == total:
		transition.status = "delivered"
		transition.failedAt = nil
	case delivered > 0:
		transition.status = "partially_delivered"
		if terminalFailures > 0 {
			transition.errorCode = stringPointer("email_partial_delivery")
			transition.errorMessage = stringPointer("The email was delivered to only some recipients")
		}
	case terminalFailures == total:
		switch {
		case bounced == total:
			transition.status = "bounced"
			transition.errorCode = stringPointer("ses_bounce")
			transition.errorMessage = stringPointer("SES reported a bounce for every recipient")
		case rejected == total:
			transition.status = "rejected"
			transition.errorCode = stringPointer("ses_reject")
			transition.errorMessage = stringPointer("SES rejected every recipient")
		case failed == total:
			transition.status = "failed"
			transition.errorCode = stringPointer("ses_rendering_failure")
			transition.errorMessage = stringPointer("SES could not process the email for any recipient")
		default:
			transition.status = "partially_failed"
			transition.errorCode = stringPointer("email_mixed_recipient_failures")
			transition.errorMessage = stringPointer("Recipients ended in different failure states")
		}
	case terminalFailures > 0:
		transition.status = "partially_failed"
		transition.errorCode = stringPointer("email_partial_failure")
		transition.errorMessage = stringPointer("At least one recipient failed while others remain unresolved")
	case counts[recipientStatusDelayed] > 0:
		transition.status = "delayed"
		transition.errorCode = stringPointer("ses_delivery_delay")
		transition.errorMessage = stringPointer("SES reported a delivery delay for at least one recipient")
	case counts[recipientStatusSubmitted] > 0:
		transition.status = "submitted"
	default:
		transition.status = fallbackStatus
	}
	return transition
}
