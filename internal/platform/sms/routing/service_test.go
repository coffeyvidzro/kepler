package routing_test

import (
	"context"
	"errors"
	"testing"

	leamoutsms "github.com/coffeyvidzro/dugble/server/internal/adapters/leamout/sms"
	runnagesms "github.com/coffeyvidzro/dugble/server/internal/adapters/runnage/sms"
	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
	"github.com/coffeyvidzro/dugble/server/internal/platform/sms/routing"
)

func TestServiceOrdersProvidersByPriority(t *testing.T) {
	t.Parallel()

	router, err := routing.NewService(routing.Config{Routes: []routing.Route{
		{ProviderID: leamoutsms.ProviderID, DestinationCountry: platformsms.CountryNigeria, Priority: 2, Enabled: true},
		{ProviderID: runnagesms.ProviderID, DestinationCountry: platformsms.CountryNigeria, Priority: 1, Enabled: true},
	}}, routing.NewPriorityStrategy(), platformsms.IsSupportedDestinationCountry)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	providerIDs, err := router.Route(context.Background(), platformsms.CountryNigeria)
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if len(providerIDs) != 2 {
		t.Fatalf("Route() count = %d, want 2", len(providerIDs))
	}
	if providerIDs[0] != runnagesms.ProviderID || providerIDs[1] != leamoutsms.ProviderID {
		t.Fatalf("Route() order = %q, %q", providerIDs[0], providerIDs[1])
	}
}

func TestSMSServiceFallsBackAfterDefinitiveRejection(t *testing.T) {
	t.Parallel()

	primary, err := runnagesms.NewProviderWithConfig(runnagesms.Config{
		SendMode: runnagesms.SendModeRejected,
	})
	if err != nil {
		t.Fatalf("NewProviderWithConfig() error = %v", err)
	}
	secondary := leamoutsms.NewProvider()
	router := newTestRouter(t, primary, secondary)
	sender, err := platformsms.NewService(router, primary, secondary)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	response, err := sender.Send(context.Background(), validNigeriaRequest())
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if response.ProviderID != leamoutsms.ProviderID {
		t.Fatalf("Send() provider = %q, want %q", response.ProviderID, leamoutsms.ProviderID)
	}
}

func TestSMSServiceStopsAfterUncertainOutcome(t *testing.T) {
	t.Parallel()

	primary, err := runnagesms.NewProviderWithConfig(runnagesms.Config{
		SendMode: runnagesms.SendModeUncertain,
	})
	if err != nil {
		t.Fatalf("NewProviderWithConfig() error = %v", err)
	}
	secondary := leamoutsms.NewProvider()
	router := newTestRouter(t, primary, secondary)
	sender, err := platformsms.NewService(router, primary, secondary)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	response, err := sender.Send(context.Background(), validNigeriaRequest())
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

func TestServiceRejectsUnsupportedRouteCountry(t *testing.T) {
	t.Parallel()

	_, err := routing.NewService(routing.Config{Routes: []routing.Route{
		{ProviderID: "example", DestinationCountry: "ZA", Priority: 1, Enabled: true},
	}}, routing.NewPriorityStrategy(), platformsms.IsSupportedDestinationCountry)
	if !errors.Is(err, routing.ErrUnsupportedCountry) {
		t.Fatalf("NewService() error = %v, want %v", err, routing.ErrUnsupportedCountry)
	}
}

func TestSMSServiceRejectsMissingRoutedProvider(t *testing.T) {
	t.Parallel()

	primary := leamoutsms.NewProvider()
	router, err := routing.NewService(routing.Config{Routes: []routing.Route{
		{ProviderID: primary.ID(), DestinationCountry: platformsms.CountryNigeria, Priority: 1, Enabled: true},
		{ProviderID: runnagesms.ProviderID, DestinationCountry: platformsms.CountryNigeria, Priority: 2, Enabled: true},
	}}, routing.NewPriorityStrategy(), platformsms.IsSupportedDestinationCountry)
	if err != nil {
		t.Fatalf("routing.NewService() error = %v", err)
	}
	if _, err := platformsms.NewService(router, primary); !errors.Is(err, platformsms.ErrProviderNotRegistered) {
		t.Fatalf("NewService() error = %v, want %v", err, platformsms.ErrProviderNotRegistered)
	}
}

func newTestRouter(
	t *testing.T,
	primary platformsms.Provider,
	secondary platformsms.Provider,
) *routing.Service {
	t.Helper()
	router, err := routing.NewService(routing.Config{Routes: []routing.Route{
		{ProviderID: primary.ID(), DestinationCountry: platformsms.CountryNigeria, Priority: 1, Enabled: true},
		{ProviderID: secondary.ID(), DestinationCountry: platformsms.CountryNigeria, Priority: 2, Enabled: true},
	}}, routing.NewPriorityStrategy(), platformsms.IsSupportedDestinationCountry)
	if err != nil {
		t.Fatalf("routing.NewService() error = %v", err)
	}
	return router
}

func validNigeriaRequest() platformsms.SendRequest {
	return platformsms.SendRequest{
		Reference:          "message-1",
		To:                 "+2348000000001",
		From:               "Dugble",
		Message:            "Hello",
		DestinationCountry: platformsms.CountryNigeria,
	}
}
