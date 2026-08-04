package ses

import "errors"

var (
	ErrMessageTooLarge           = errors.New("email exceeds the SES raw message size limit")
	ErrReservedHeader            = errors.New("email header is reserved by the SES integration")
	ErrUnsupportedAttachmentPath = errors.New("attachment paths are not supported by the SES integration")
)
