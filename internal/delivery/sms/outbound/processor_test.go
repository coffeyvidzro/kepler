package smsdelivery

import (
	"testing"

	"github.com/google/uuid"

	smsmodule "github.com/coffeyvidzro/dugble/server/internal/modules/sms"
)

func TestSupportedDeliveryRoutesFiltersAccountsProvidersAndDuplicates(t *testing.T) {
	t.Parallel()

	routes := []smsmodule.DeliveryRoute{
		{SenderID: uuid.New(), Provider: "mnotify", ProviderAccount: "default"},
		{SenderID: uuid.New(), Provider: "MNOTIFY", ProviderAccount: "default"},
		{SenderID: uuid.New(), Provider: "moolre", ProviderAccount: "secondary"},
		{SenderID: uuid.New(), Provider: "unknown", ProviderAccount: "default"},
		{SenderID: uuid.New(), Provider: "moolre", ProviderAccount: "default"},
	}

	filtered := supportedDeliveryRoutes(routes, []string{"mnotify", "moolre"})
	if len(filtered) != 2 {
		t.Fatalf("supportedDeliveryRoutes() length = %d, want 2", len(filtered))
	}
	if filtered[0].Provider != "mnotify" || filtered[1].Provider != "moolre" {
		t.Fatalf("supportedDeliveryRoutes() = %#v", filtered)
	}
}
