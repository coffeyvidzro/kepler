package webhooks

import (
	"reflect"
	"testing"

	platformwebhook "github.com/coffeyvidzro/dugble/server/internal/platform/webhook"
)

func TestValidateCreateEndpoint(t *testing.T) {
	value, err := validateCreateEndpoint(CreateEndpointRequest{
		URL: " https://example.com/hooks/dugble ",
		SubscribedEvents: []string{
			platformwebhook.EventSMSDelivered,
			platformwebhook.EventSMSDelivered,
			platformwebhook.EventSMSFailed,
		},
	})
	if err != nil {
		t.Fatalf("validateCreateEndpoint() error = %v", err)
	}
	if value.URL != "https://example.com/hooks/dugble" {
		t.Fatalf("URL = %q", value.URL)
	}
	wantEvents := []string{platformwebhook.EventSMSDelivered, platformwebhook.EventSMSFailed}
	if !reflect.DeepEqual(value.SubscribedEvents, wantEvents) {
		t.Fatalf("events = %#v, want %#v", value.SubscribedEvents, wantEvents)
	}
	if !value.Enabled {
		t.Fatal("new endpoint should be enabled")
	}
}

func TestValidateCreateEndpointRejectsInsecureURL(t *testing.T) {
	_, err := validateCreateEndpoint(CreateEndpointRequest{
		URL:              "http://example.com/hooks/dugble",
		SubscribedEvents: []string{platformwebhook.EventSMSDelivered},
	})
	if err == nil {
		t.Fatal("validateCreateEndpoint() error = nil, want error")
	}
}

func TestValidateCreateEndpointRejectsUnsupportedEvent(t *testing.T) {
	_, err := validateCreateEndpoint(CreateEndpointRequest{
		URL:              "https://example.com/hooks/dugble",
		SubscribedEvents: []string{"sms.unknown"},
	})
	if err == nil {
		t.Fatal("validateCreateEndpoint() error = nil, want error")
	}
}
