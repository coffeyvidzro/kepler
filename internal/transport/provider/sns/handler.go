package sns

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v5"

	awsses "github.com/coffeyvidzro/dugble/server/internal/integration/aws/ses"
	awssns "github.com/coffeyvidzro/dugble/server/internal/integration/aws/sns"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

type envelopeVerifier interface {
	Verify(context.Context, awssns.Envelope) error
}

type notificationIngestor interface {
	Ingest(context.Context, awssns.Envelope) error
}

type Handler struct {
	verifier  envelopeVerifier
	confirmer awssns.SubscriptionConfirmer
	ingestor  notificationIngestor
}

func NewHandler(verifier envelopeVerifier, confirmer awssns.SubscriptionConfirmer, ingestor notificationIngestor) *Handler {
	return &Handler{verifier: verifier, confirmer: confirmer, ingestor: ingestor}
}

func (h *Handler) ReceiveSES(c *echo.Context) error {
	envelope, err := parseEnvelopeRequest(c)
	if err != nil {
		slog.WarnContext(c.Request().Context(), "rejected AWS SNS request", "error", err)
		return httputil.Error(c, err)
	}
	slog.InfoContext(c.Request().Context(), "accepted AWS SNS request for verification",
		"sns_message_type", envelope.Type,
		"sns_message_id", envelope.MessageID,
		"sns_topic_arn", envelope.TopicARN,
	)
	if h == nil || h.verifier == nil {
		return httputil.Error(c, apperrors.NewServiceUnavailable("SNS verification is not configured", nil))
	}
	if err := h.verifier.Verify(c.Request().Context(), envelope); err != nil {
		slog.WarnContext(c.Request().Context(), "AWS SNS request failed verification",
			"sns_message_type", envelope.Type,
			"sns_message_id", envelope.MessageID,
			"sns_topic_arn", envelope.TopicARN,
			"error", err,
		)
		return httputil.Error(c, mapIntegrationError(err))
	}

	switch envelope.Type {
	case awssns.TypeSubscriptionConfirmation:
		if h.confirmer == nil {
			return httputil.Error(c, apperrors.NewServiceUnavailable("SNS subscription confirmation is not configured", nil))
		}
		if err := h.confirmer.Confirm(c.Request().Context(), envelope); err != nil {
			return httputil.Error(c, mapIntegrationError(err))
		}
		return c.NoContent(http.StatusNoContent)

	case awssns.TypeNotification:
		if h.ingestor == nil {
			return httputil.Error(c, apperrors.NewServiceUnavailable("SNS notification ingestion is not configured", nil))
		}
		if err := h.ingestor.Ingest(c.Request().Context(), envelope); err != nil {
			slog.WarnContext(c.Request().Context(), "AWS SNS SES notification ingestion failed",
				"sns_message_id", envelope.MessageID,
				"sns_topic_arn", envelope.TopicARN,
				"error", err,
			)
			if errors.Is(err, awsses.ErrInvalidEvent) {
				return httputil.Error(c, apperrors.NewBadRequest("SNS notification does not contain a valid SES event"))
			}
			return httputil.Error(c, apperrors.NewServiceUnavailable("Unable to accept SNS notification", err))
		}
		return c.NoContent(http.StatusNoContent)

	case awssns.TypeUnsubscribeConfirmation:
		return c.NoContent(http.StatusNoContent)

	default:
		return httputil.Error(c, apperrors.NewBadRequest("Unsupported SNS message type"))
	}
}
