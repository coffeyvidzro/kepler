package webhooks

import (
	"net/url"
	"slices"
	"strings"

	platformwebhook "github.com/coffeyvidzro/dugble/server/internal/platform/webhook"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

const maxWebhookURLLength = 2048

type validatedEndpoint struct {
	URL              string
	Enabled          bool
	SubscribedEvents []string
}

func validateCreateEndpoint(req CreateEndpointRequest) (validatedEndpoint, error) {
	return validateEndpoint(req.URL, true, req.SubscribedEvents)
}

func validateUpdateEndpoint(current Endpoint, req UpdateEndpointRequest) (validatedEndpoint, error) {
	value := validatedEndpoint{
		URL:              current.URL,
		Enabled:          current.Enabled,
		SubscribedEvents: current.SubscribedEvents,
	}
	if req.URL != nil {
		value.URL = *req.URL
	}
	if req.Enabled != nil {
		value.Enabled = *req.Enabled
	}
	if req.SubscribedEvents != nil {
		value.SubscribedEvents = *req.SubscribedEvents
	}
	return validateEndpoint(value.URL, value.Enabled, value.SubscribedEvents)
}

func validateEndpoint(rawURL string, enabled bool, events []string) (validatedEndpoint, error) {
	rawURL = strings.TrimSpace(rawURL)
	if len(rawURL) > maxWebhookURLLength {
		return validatedEndpoint{}, apperrors.NewBadRequest("Webhook URL is too long")
	}
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return validatedEndpoint{}, apperrors.NewBadRequest(
			"Webhook URL must be an absolute HTTPS URL without user information",
		)
	}

	normalizedEvents := make([]string, 0, len(events))
	for _, event := range events {
		event = strings.TrimSpace(event)
		if event == "" || slices.Contains(normalizedEvents, event) {
			continue
		}
		if !platformwebhook.IsSubscribableEventType(event) {
			return validatedEndpoint{}, apperrors.NewBadRequest("Unsupported webhook event: " + event)
		}
		normalizedEvents = append(normalizedEvents, event)
	}
	if len(normalizedEvents) == 0 {
		return validatedEndpoint{}, apperrors.NewBadRequest("At least one subscribed event is required")
	}
	return validatedEndpoint{URL: rawURL, Enabled: enabled, SubscribedEvents: normalizedEvents}, nil
}
