package routing_test

import (
	"errors"
	"testing"

	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
	"github.com/coffeyvidzro/dugble/server/internal/platform/sms/routing"
)

func TestDefaultConfigStagesGhanaProvidersByPriority(t *testing.T) {
	t.Parallel()

	config := routing.DefaultConfig()
	var mnotifyRoute, moolreRoute *routing.Route
	for index := range config.Routes {
		route := &config.Routes[index]
		if route.DestinationCountry != platformsms.CountryGhana {
			continue
		}
		switch route.ProviderID {
		case "mnotify":
			mnotifyRoute = route
		case "moolre":
			moolreRoute = route
		}
	}
	if mnotifyRoute == nil || mnotifyRoute.Priority != 1 || !mnotifyRoute.Enabled {
		t.Fatalf("mNotify route = %#v", mnotifyRoute)
	}
	if moolreRoute == nil || moolreRoute.Priority != 2 || moolreRoute.Enabled {
		t.Fatalf("Moolre route = %#v", moolreRoute)
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
				{ProviderID: "leamout", DestinationCountry: platformsms.CountryKenya, Priority: 0, Enabled: true},
			},
			target: routing.ErrInvalidPriority,
		},
		{
			name: "duplicate provider",
			routes: []routing.Route{
				{ProviderID: "leamout", DestinationCountry: platformsms.CountryKenya, Priority: 1, Enabled: true},
				{ProviderID: "leamout", DestinationCountry: platformsms.CountryKenya, Priority: 2, Enabled: false},
			},
			target: routing.ErrDuplicateProvider,
		},
		{
			name: "duplicate priority",
			routes: []routing.Route{
				{ProviderID: "leamout", DestinationCountry: platformsms.CountryKenya, Priority: 1, Enabled: true},
				{ProviderID: "runnage", DestinationCountry: platformsms.CountryKenya, Priority: 1, Enabled: true},
			},
			target: routing.ErrDuplicatePriority,
		},
		{
			name: "unsupported country",
			routes: []routing.Route{
				{ProviderID: "leamout", DestinationCountry: "ZA", Priority: 1, Enabled: true},
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
