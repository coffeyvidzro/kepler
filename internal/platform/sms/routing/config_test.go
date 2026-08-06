package routing_test

import (
	"errors"
	"testing"

	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
	"github.com/coffeyvidzro/dugble/server/internal/platform/sms/routing"
)

func TestDefaultConfigPrioritizesCountryProviders(t *testing.T) {
	t.Parallel()

	config := routing.DefaultConfig()
	var ghana, nigeria []routing.Route
	for _, route := range config.Routes {
		if !route.Enabled {
			continue
		}
		switch route.DestinationCountry {
		case platformsms.CountryGhana:
			ghana = append(ghana, route)
		case platformsms.CountryNigeria:
			nigeria = append(nigeria, route)
		}
	}
	if len(ghana) != 2 || ghana[0].ProviderID != "mnotify" || ghana[0].Priority != 1 || ghana[1].ProviderID != "moolre" || ghana[1].Priority != 2 {
		t.Fatalf("Ghana routes = %#v", ghana)
	}
	if len(nigeria) != 2 || nigeria[0].ProviderID != "leamout" || nigeria[0].Priority != 1 || nigeria[1].ProviderID != "runnage" || nigeria[1].Priority != 2 {
		t.Fatalf("Nigeria routes = %#v", nigeria)
	}
}

func TestDefaultProviderIDUsesHighestPriorityEnabledRoute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		country  string
		provider string
		found    bool
	}{
		{name: "Ghana", country: platformsms.CountryGhana, provider: "mnotify", found: true},
		{name: "Nigeria", country: platformsms.CountryNigeria, provider: "leamout", found: true},
		{name: "unsupported", country: "KE", found: false},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			provider, found := routing.DefaultProviderID(test.country)
			if found != test.found || provider != test.provider {
				t.Fatalf(
					"DefaultProviderID(%q) = %q, %t; want %q, %t",
					test.country,
					provider,
					found,
					test.provider,
					test.found,
				)
			}
		})
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
			name: "malformed country",
			routes: []routing.Route{
				{ProviderID: "leamout", DestinationCountry: "NGA", Priority: 1, Enabled: true},
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
