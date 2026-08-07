package broadcastexecution

import (
	"errors"
	"strings"
	"time"

	messagetemplate "github.com/coffeyvidzro/dugble/server/internal/modules/messagetemplate"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

const maxFanoutFailureMessageRunes = 255

type RetryPolicy struct {
	MaxAttempts int32
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 8,
		BaseDelay:   time.Minute,
		MaxDelay:    30 * time.Minute,
	}
}

func (policy RetryPolicy) normalized() RetryPolicy {
	defaults := DefaultRetryPolicy()
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = defaults.MaxAttempts
	}
	if policy.BaseDelay <= 0 {
		policy.BaseDelay = defaults.BaseDelay
	}
	if policy.MaxDelay < policy.BaseDelay {
		policy.MaxDelay = defaults.MaxDelay
	}
	return policy
}

func (policy RetryPolicy) delay(attemptCount int32) time.Duration {
	policy = policy.normalized()
	delay := policy.BaseDelay
	for attempt := int32(0); attempt < attemptCount; attempt++ {
		if delay >= policy.MaxDelay/2 {
			return policy.MaxDelay
		}
		delay *= 2
	}
	if delay > policy.MaxDelay {
		return policy.MaxDelay
	}
	return delay
}

type fanoutFailure struct {
	code      string
	message   string
	retryable bool
	cause     error
}

func classifyRenderFailure(err error) fanoutFailure {
	if errors.Is(err, messagetemplate.ErrVersionNotFound) {
		return fanoutFailure{
			code:      apperrors.CodeNotFound,
			message:   "Pinned template version not found",
			retryable: false,
			cause:     err,
		}
	}
	message := err.Error()
	if strings.Contains(message, "render pinned message template version") {
		return fanoutFailure{
			code:      apperrors.CodeBadRequest,
			message:   "Pinned template version could not be rendered",
			retryable: false,
			cause:     err,
		}
	}
	return classifyFanoutFailure(err)
}

func classifyFanoutFailure(err error) fanoutFailure {
	code := apperrors.CodeOf(err)
	message := safeFailureMessage(apperrors.MessageOf(err))
	retryable := true
	switch code {
	case apperrors.CodeBadRequest,
		apperrors.CodeForbidden,
		apperrors.CodeNotFound,
		apperrors.CodePayloadTooLarge,
		apperrors.CodeUnprocessableEntity:
		retryable = false
	case apperrors.CodePaymentRequired,
		apperrors.CodeConflict,
		apperrors.CodeTooManyRequests,
		apperrors.CodeInternal,
		apperrors.CodeServiceUnavailable:
		retryable = true
	}
	return fanoutFailure{code: code, message: message, retryable: retryable, cause: err}
}

func safeFailureMessage(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "Recipient fanout failed"
	}
	runes := []rune(message)
	if len(runes) > maxFanoutFailureMessageRunes {
		message = string(runes[:maxFanoutFailureMessageRunes])
	}
	return message
}
