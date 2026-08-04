package routing

import (
	"context"
	"errors"
	"testing"

	"github.com/coffeyvidzro/dugble/server/internal/integration/sms"
)

func TestRouteSelectsProviderByDestinationCountry(t *testing.T) {
	ghanaProvider := countryProvider{id: "ghana-provider"}
	nigeriaProvider := countryProvider{id: "nigeria-provider"}
	service, err := NewService(
		Config{Routes: []Route{
			{ProviderID: ghanaProvider.id, DestinationCountry: sms.CountryGhana, Enabled: true},
			{ProviderID: nigeriaProvider.id, DestinationCountry: sms.CountryNigeria, Enabled: true},
		}},
		ghanaProvider,
		nigeriaProvider,
	)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	provider, err := service.Route(context.Background(), sms.SendRequest{
		To:                 "+233241234567",
		From:               "DUGBLE",
		Message:            "hello",
		DestinationCountry: sms.CountryGhana,
	})
	if err != nil {
		t.Fatalf("Route returned error: %v", err)
	}
	if provider == nil || provider.ID() != ghanaProvider.id {
		t.Fatalf("Route provider = %#v, want %q", provider, ghanaProvider.id)
	}
}

func TestRouteDoesNotCrossCountries(t *testing.T) {
	ghanaProvider := countryProvider{id: "ghana-provider"}
	service, err := NewService(
		Config{Routes: []Route{
			{ProviderID: ghanaProvider.id, DestinationCountry: sms.CountryGhana, Enabled: true},
		}},
		ghanaProvider,
	)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	_, err = service.Route(context.Background(), sms.SendRequest{
		To:                 "+2348012345678",
		From:               "DUGBLE",
		Message:            "hello",
		DestinationCountry: sms.CountryNigeria,
	})
	if !errors.Is(err, sms.ErrNoProviderAvailable) {
		t.Fatalf("Route error = %v, want ErrNoProviderAvailable", err)
	}
}

func TestConfigRejectsMultipleProvidersForCountry(t *testing.T) {
	config := Config{Routes: []Route{
		{ProviderID: "primary", DestinationCountry: sms.CountryGhana, Enabled: true},
		{ProviderID: "secondary", DestinationCountry: sms.CountryGhana, Enabled: true},
	}}
	if !errors.Is(config.Validate(), ErrDuplicateCountry) {
		t.Fatalf("Validate error = %v, want ErrDuplicateCountry", config.Validate())
	}
}

func TestConfigRejectsProviderAcrossMultipleCountries(t *testing.T) {
	config := Config{Routes: []Route{
		{ProviderID: "shared", DestinationCountry: sms.CountryGhana, Enabled: true},
		{ProviderID: "shared", DestinationCountry: sms.CountryNigeria, Enabled: true},
	}}
	if !errors.Is(config.Validate(), ErrDuplicateProvider) {
		t.Fatalf("Validate error = %v, want ErrDuplicateProvider", config.Validate())
	}
}

type countryProvider struct{ id string }

func (p countryProvider) ID() string { return p.id }

func (countryProvider) Send(context.Context, sms.SendRequest) (*sms.SendResponse, error) {
	return nil, errors.New("not implemented")
}

func (countryProvider) CheckStatus(context.Context, string) (*sms.StatusResponse, error) {
	return nil, errors.New("not implemented")
}
