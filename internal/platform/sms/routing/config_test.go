package routing_test

import (
	"errors"
	"testing"

	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
	"github.com/coffeyvidzro/dugble/server/internal/platform/sms/routing"
)

func TestDefaultConfigPrioritizesGhanaAndNigeriaProviders(t *testing.T) {
	t.Parallel()

	config := routing.DefaultConfig()
	assertRoute := func(country, provider string, priority int) {
		t.Helper()
		for _, route := range config.Routes {
			if route.DestinationCountry == country && route.ProviderID == provider {
				if route.Priority != priority || !route.Enabled {
					t.Fatalf("route %s/%s = %#v", country, provider, route)
				}
				return
			}
		}
		t.Fatalf("route %s/%s not found", country, provider)
	}

	assertRoute(platformsms.CountryGhana, "mnotify", 1)
	assertRoute(platformsms.CountryGhana, "moolre", 2)
	assertRoute(platformsms.CountryNigeria, "leamout", 1)
	assertRoute(platformsms.CountryNigeria, "runnage", 2)

	for _, route := range config.Routes {
		if route.DestinationCountry == "KE" || route.ProviderID == "celcom" || route.ProviderID == "arkesel" {
			t.Fatalf("obsolete default route = %#v", route)
		}
	}
}

func TestConfigRejectsInvalidRoutes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		routes []routing.Route
		target error
	}{
		{
			name: "invalid priority",
			routes: []routing.Route{
				{ProviderID: "leamout", DestinationCountry: platformsms.CountryNigeria, Priority: 0, Enabled: true},
			},
			target: routing.ErrInvalidPriority,
		},
		{
			name: "duplicate provider",
			routes: []routing.Route{
				{ProviderID: "leamout", DestinationCountry: platformsms.CountryNigeria, Priority: 1, Enabled: true},
				{ProviderID: "leamout", DestinationCountry: platformsms.CountryNigeria, Priority: 2, Enabled: false},
			},
			target: routing.ErrDuplicateProvider,
		},
		{
			name: "duplicate priority",
			routes: []routing.Route{
				{ProviderID: "leamout", DestinationCountry: platformsms.CountryNigeria, Priority: 1, Enabled: true},
				{ProviderID: "runnage", DestinationCountry: platformsms.CountryNigeria, Priority: 1, Enabled: true},
			},
			target: routing.ErrDuplicatePriority,
		},
		{
			name: "unsupported country",
			routes: []routing.Route{
				{ProviderID: "leamout", DestinationCountry: "KE", Priority: 1, Enabled: true},
			},
			target: routing.ErrInvalidCountryCode,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := (routing.Config{Routes: test.routes}).Validate()
			if !errors.Is(err, test.target) {
				t.Fatalf("Validate() error = %v, want %v", err, test.target)
			}
		})
	}
}
