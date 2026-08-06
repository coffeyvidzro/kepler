package smsdelivery

import (
	"testing"

	"github.com/google/uuid"

	platformrouting "github.com/coffeyvidzro/dugble/server/internal/platform/messaging/routing"
)

func TestSupportedDeliveryRoutesFiltersAccountsProvidersAndDuplicates(t *testing.T) {
	t.Parallel()

	routes := []platformrouting.Route{
		{SenderProviderBindingID: uuid.New(), Provider: "mnotify", ProviderAccount: "default"},
		{SenderProviderBindingID: uuid.New(), Provider: "MNOTIFY", ProviderAccount: "default"},
		{SenderProviderBindingID: uuid.New(), Provider: "moolre", ProviderAccount: "secondary"},
		{SenderProviderBindingID: uuid.New(), Provider: "unknown", ProviderAccount: "default"},
		{SenderProviderBindingID: uuid.New(), Provider: "moolre", ProviderAccount: "default"},
	}

	filtered := supportedDeliveryRoutes(routes, []string{"mnotify", "moolre"})
	if len(filtered) != 2 {
		t.Fatalf("supportedDeliveryRoutes() length = %d, want 2", len(filtered))
	}
	if filtered[0].Provider != "mnotify" || filtered[1].Provider != "moolre" {
		t.Fatalf("supportedDeliveryRoutes() = %#v", filtered)
	}
}
