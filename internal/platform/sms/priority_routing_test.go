package sms_test

import (
	"context"
	"errors"
	"testing"

	leamoutsms "github.com/coffeyvidzro/dugble/server/internal/adapters/leamout/sms"
	runnagesms "github.com/coffeyvidzro/dugble/server/internal/adapters/runnage/sms"
	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

func TestRoutingServiceOrdersCandidatesByPriority(t *testing.T) {
	t.Parallel()

	leamout := leamoutsms.NewProvider()
	runnage := runnagesms.NewProvider()
	router, err := platformsms.NewRoutingService(platformsms.RoutingConfig{Routes: []platformsms.Route{
		{ProviderID: leamout.ID(), DestinationCountry: platformsms.CountryKenya, Priority: 2, Enabled: true},
		{ProviderID: runnage.ID(), DestinationCountry: platformsms.CountryKenya, Priority: 1, Enabled: true},
	}}, leamout, runnage)
	if err != nil {
		t.Fatalf("NewRoutingService() error = %v", err)
	}

	request := validKenyaRequest()
	candidates, err := router.Candidates(context.Background(), request)
	if err != nil {
		t.Fatalf("Candidates() error = %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("Candidates() count = %d, want 2", len(candidates))
	}
	if candidates[0].ID() != runnagesms.ProviderID || candidates[1].ID() != leamoutsms.ProviderID {
		t.Fatalf("Candidates() order = %q, %q", candidates[0].ID(), candidates[1].ID())
	}

	primary, err := router.Route(context.Background(), request)
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if primary.ID() != runnagesms.ProviderID {
		t.Fatalf("Route() provider = %q, want %q", primary.ID(), runnagesms.ProviderID)
	}
}

func TestServiceFallsBackAfterDefinitiveRejection(t *testing.T) {
	t.Parallel()

	primary, err := runnagesms.NewProviderWithConfig(runnagesms.Config{
		SendMode: runnagesms.SendModeRejected,
	})
	if err != nil {
		t.Fatalf("NewProviderWithConfig() error = %v", err)
	}
	secondary := leamoutsms.NewProvider()
	router, err := platformsms.NewRoutingService(platformsms.RoutingConfig{Routes: []platformsms.Route{
		{ProviderID: primary.ID(), DestinationCountry: platformsms.CountryKenya, Priority: 1, Enabled: true},
		{ProviderID: secondary.ID(), DestinationCountry: platformsms.CountryKenya, Priority: 2, Enabled: true},
	}}, primary, secondary)
	if err != nil {
		t.Fatalf("NewRoutingService() error = %v", err)
	}
	service, err := platformsms.NewService(router)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	response, err := service.Send(context.Background(), validKenyaRequest())
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if response.ProviderID != leamoutsms.ProviderID {
		t.Fatalf("Send() provider = %q, want %q", response.ProviderID, leamoutsms.ProviderID)
	}
}

func TestServiceStopsAfterUncertainOutcome(t *testing.T) {
	t.Parallel()

	primary, err := runnagesms.NewProviderWithConfig(runnagesms.Config{
		SendMode: runnagesms.SendModeUncertain,
	})
	if err != nil {
		t.Fatalf("NewProviderWithConfig() error = %v", err)
	}
	secondary := leamoutsms.NewProvider()
	router, err := platformsms.NewRoutingService(platformsms.RoutingConfig{Routes: []platformsms.Route{
		{ProviderID: primary.ID(), DestinationCountry: platformsms.CountryKenya, Priority: 1, Enabled: true},
		{ProviderID: secondary.ID(), DestinationCountry: platformsms.CountryKenya, Priority: 2, Enabled: true},
	}}, primary, secondary)
	if err != nil {
		t.Fatalf("NewRoutingService() error = %v", err)
	}
	service, err := platformsms.NewService(router)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	response, err := service.Send(context.Background(), validKenyaRequest())
	if response != nil {
		t.Fatalf("Send() response = %#v, want nil", response)
	}
	var sendErr *platformsms.SendError
	if !errors.As(err, &sendErr) {
		t.Fatalf("Send() error = %T, want *sms.SendError", err)
	}
	if len(sendErr.Attempts) != 1 || sendErr.Attempts[0].ProviderID != runnagesms.ProviderID {
		t.Fatalf("Send() attempts = %#v", sendErr.Attempts)
	}
}

func TestRoutingConfigRejectsInvalidPriorityAndDuplicates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		routes []platformsms.Route
		target error
	}{
		{
			name: "invalid priority",
			routes: []platformsms.Route{
				{ProviderID: "leamout", DestinationCountry: platformsms.CountryKenya, Priority: 0, Enabled: true},
			},
			target: platformsms.ErrInvalidRoutePriority,
		},
		{
			name: "duplicate provider route",
			routes: []platformsms.Route{
				{ProviderID: "leamout", DestinationCountry: platformsms.CountryKenya, Priority: 1, Enabled: true},
				{ProviderID: "leamout", DestinationCountry: platformsms.CountryKenya, Priority: 2, Enabled: false},
			},
			target: platformsms.ErrDuplicateRoute,
		},
		{
			name: "duplicate country priority",
			routes: []platformsms.Route{
				{ProviderID: "leamout", DestinationCountry: platformsms.CountryKenya, Priority: 1, Enabled: true},
				{ProviderID: "runnage", DestinationCountry: platformsms.CountryKenya, Priority: 1, Enabled: true},
			},
			target: platformsms.ErrDuplicateRoutePriority,
		},
		{
			name: "unsupported country",
			routes: []platformsms.Route{
				{ProviderID: "leamout", DestinationCountry: "ZA", Priority: 1, Enabled: true},
			},
			target: platformsms.ErrInvalidCountryCode,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := (platformsms.RoutingConfig{Routes: test.routes}).Validate()
			if !errors.Is(err, test.target) {
				t.Fatalf("Validate() error = %v, want %v", err, test.target)
			}
		})
	}
}

func TestSupportedDestinationCountryUsesPhoneResolutionCatalog(t *testing.T) {
	t.Parallel()

	if !platformsms.IsSupportedDestinationCountry("gh") {
		t.Fatal("IsSupportedDestinationCountry(gh) = false")
	}
	if platformsms.IsSupportedDestinationCountry("ZA") {
		t.Fatal("IsSupportedDestinationCountry(ZA) = true")
	}

	destinations := platformsms.SupportedDestinations()
	if len(destinations) == 0 {
		t.Fatal("SupportedDestinations() returned no destinations")
	}
	destinations[0].CountryCode = "XX"
	if !platformsms.IsSupportedDestinationCountry(platformsms.CountryGhana) {
		t.Fatal("SupportedDestinations() exposed mutable internal state")
	}
}

func validKenyaRequest() platformsms.SendRequest {
	return platformsms.SendRequest{
		Reference:          "message-1",
		To:                 "+254700000001",
		From:               "Dugble",
		Message:            "Hello",
		DestinationCountry: platformsms.CountryKenya,
	}
}
