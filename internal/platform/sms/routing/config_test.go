package routing_test

import (
	"errors"
	"testing"

	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
	"github.com/coffeyvidzro/dugble/server/internal/platform/sms/routing"
)

func TestDefaultConfigPrioritizesGhanaProviders(t *testing.T) {
	t.Parallel()

	config := routing.DefaultConfig()
	var ghana []routing.Route
	for _, route := range config.Routes {
		if route.Enabled && route.DestinationCountry == platformsms.CountryGhana {
			ghana = append(ghana, route)
		}
	}
	if len(ghana) != 2 {
		t.Fatalf("Ghana route count = %d, want 2", len(ghana))
	}
	if ghana[0].ProviderID != "mnotify" || ghana[0].Priority != 1 {
		t.Fatalf("Ghana primary route = %#v", ghana[0])
	}
	if ghana[1].ProviderID != "moolre" || ghana[1].Priority != 2 {
		t.Fatalf("Ghana secondary route = %#v", ghana[1])
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
