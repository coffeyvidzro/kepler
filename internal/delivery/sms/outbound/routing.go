package smsdelivery

import (
	"errors"

	smsapi "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

type safeFallbackError interface {
	error
	SafeToFallback() bool
}

func shouldFinalizeAfterSendError(err error) bool {
	if err == nil {
		return false
	}
	var validationErr *smsapi.ValidationError
	if errors.As(err, &validationErr) {
		return true
	}
	if errors.Is(err, smsapi.ErrNoProviderAvailable) || errors.Is(err, smsapi.ErrProviderNotFound) {
		return true
	}

	var sendErr *smsapi.SendError
	if errors.As(err, &sendErr) {
		if len(sendErr.Attempts) == 0 {
			return false
		}
		for _, attempt := range sendErr.Attempts {
			if !safeProviderRejection(attempt.Err) {
				return false
			}
		}
		return true
	}

	return safeProviderRejection(err)
}

func safeProviderRejection(err error) bool {
	if err == nil {
		return false
	}
	var fallbackErr safeFallbackError
	return errors.As(err, &fallbackErr) && fallbackErr.SafeToFallback()
}
