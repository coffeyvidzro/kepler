package sns

import (
	"errors"
	"fmt"
	"io"
	"mime"

	"github.com/labstack/echo/v5"

	awssns "github.com/coffeyvidzro/dugble/server/internal/integration/aws/sns"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

const maxRequestBodyBytes = 256 * 1024

func parseEnvelopeRequest(c *echo.Context) (awssns.Envelope, error) {
	mediaType, _, err := mime.ParseMediaType(c.Request().Header.Get("Content-Type"))
	if err != nil || mediaType != "text/plain" {
		return awssns.Envelope{}, apperrors.NewBadRequest("SNS requests must use text/plain content type")
	}
	body, err := io.ReadAll(io.LimitReader(c.Request().Body, maxRequestBodyBytes+1))
	if err != nil {
		return awssns.Envelope{}, apperrors.NewBadRequest("Unable to read SNS request body")
	}
	if len(body) > maxRequestBodyBytes {
		return awssns.Envelope{}, apperrors.NewPayloadTooLarge("SNS request body is too large")
	}
	envelope, err := awssns.ParseEnvelope(body)
	if err != nil {
		return awssns.Envelope{}, apperrors.NewBadRequest("Invalid SNS request body")
	}
	if headerType := c.Request().Header.Get("x-amz-sns-message-type"); headerType != "" && headerType != string(envelope.Type) {
		return awssns.Envelope{}, apperrors.NewBadRequest("SNS message type header does not match body")
	}
	return envelope, nil
}

func mapIntegrationError(err error) error {
	switch {
	case errors.Is(err, awssns.ErrInvalidEnvelope), errors.Is(err, awssns.ErrUnsupportedMessageType):
		return apperrors.NewBadRequest("Invalid SNS request")
	case errors.Is(err, awssns.ErrInvalidSignature), errors.Is(err, awssns.ErrUnsupportedSignatureVersion):
		return apperrors.NewUnauthorized("Invalid SNS signature")
	case errors.Is(err, awssns.ErrTopicNotAllowed), errors.Is(err, awssns.ErrUntrustedCertificateURL):
		return apperrors.NewForbidden("SNS notification source is not allowed")
	case errors.Is(err, awssns.ErrCertificateUnavailable), errors.Is(err, awssns.ErrConfirmationUnavailable):
		return apperrors.NewServiceUnavailable("SNS integration is temporarily unavailable", err)
	case errors.Is(err, awssns.ErrInvalidCertificate):
		return apperrors.NewUnauthorized("Invalid SNS signing certificate")
	default:
		return apperrors.NewInternal("Unable to process SNS request", fmt.Errorf("SNS integration: %w", err))
	}
}
