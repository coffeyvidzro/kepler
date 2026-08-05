package emaildelivery

import "errors"

func IsMessageNotDeliverable(err error) bool {
	return errors.Is(err, ErrMessageNotDeliverable)
}

func IsSenderDomainUnavailable(err error) bool {
	return errors.Is(err, ErrSenderDomainUnavailable)
}
