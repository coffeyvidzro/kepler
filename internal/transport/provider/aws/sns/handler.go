package sns

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"strings"

	"github.com/labstack/echo/v5"

	awsses "github.com/coffeyvidzro/dugble/server/internal/adapters/amazon/ses"
	awssns "github.com/coffeyvidzro/dugble/server/internal/adapters/amazon/sns"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

const maxRequestBodyBytes int64 = 256 * 1024

type EnvelopeVerifier interface {
	Verify(context.Context, awssns.Envelope) error
}

type NotificationIngestor interface {
	IngestSNS(context.Context, awssns.Envelope) error
}

type Handler struct {
	verifier  EnvelopeVerifier
	confirmer awssns.SubscriptionConfirmer
	ingestor  NotificationIngestor
}

func NewHandler(verifier EnvelopeVerifier, confirmer awssns.SubscriptionConfirmer, ingestor NotificationIngestor) *Handler {
	return &Handler{verifier: verifier, confirmer: confirmer, ingestor: ingestor}
}

func (handler *Handler) ReceiveSES(c *echo.Context) error {
	envelope, err := parseEnvelopeRequest(c)
	if err != nil {
		slog.WarnContext(c.Request().Context(), "rejected AWS SNS request", "error", err)
		return httputil.Error(c, err)
	}
	if handler == nil || handler.verifier == nil {
		return httputil.Error(c, apperrors.NewServiceUnavailable("SNS verification is not configured", nil))
	}
	if err := handler.verifier.Verify(c.Request().Context(), envelope); err != nil {
		return httputil.Error(c, mapIntegrationError(err))
	}
	switch envelope.Type {
	case awssns.TypeSubscriptionConfirmation:
		if handler.confirmer == nil {
			return httputil.Error(c, apperrors.NewServiceUnavailable("SNS subscription confirmation is not configured", nil))
		}
		if err := handler.confirmer.Confirm(c.Request().Context(), envelope); err != nil {
			return httputil.Error(c, mapIntegrationError(err))
		}
		return httputil.NoContent(c)
	case awssns.TypeNotification:
		if handler.ingestor == nil {
			return httputil.Error(c, apperrors.NewServiceUnavailable("SNS notification ingestion is not configured", nil))
		}
		if err := handler.ingestor.IngestSNS(c.Request().Context(), envelope); err != nil {
			if errors.Is(err, awsses.ErrInvalidEvent) {
				return httputil.Error(c, apperrors.NewBadRequest("SNS notification does not contain a valid SES event"))
			}
			return httputil.Error(c, apperrors.NewServiceUnavailable("Unable to accept SNS notification", err))
		}
		return httputil.NoContent(c)
	case awssns.TypeUnsubscribeConfirmation:
		return httputil.NoContent(c)
	default:
		return httputil.Error(c, apperrors.NewBadRequest("Unsupported SNS message type"))
	}
}

func parseEnvelopeRequest(c *echo.Context) (awssns.Envelope, error) {
	mediaType, _, err := mime.ParseMediaType(c.Request().Header.Get("Content-Type"))
	if err != nil || mediaType != "text/plain" {
		return awssns.Envelope{}, apperrors.NewBadRequest("SNS requests must use text/plain content type")
	}
	body, err := httputil.ReadBody(c, maxRequestBodyBytes)
	if err != nil {
		return awssns.Envelope{}, err
	}
	envelope, err := awssns.ParseEnvelope(body)
	if err != nil {
		return awssns.Envelope{}, apperrors.NewBadRequest("Invalid SNS request body")
	}
	if headerType := strings.TrimSpace(c.Request().Header.Get("x-amz-sns-message-type")); headerType != "" && headerType != string(envelope.Type) {
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
