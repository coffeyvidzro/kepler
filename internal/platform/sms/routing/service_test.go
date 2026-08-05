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

	leamout := leamoutsms.NewProvider()
	runnage := runnagesms.NewProvider()
	router, err := routing.NewService(routing.Config{Routes: []routing.Route{
		{ProviderID: leamout.ID(), DestinationCountry: platformsms.CountryKenya, Priority: 2, Enabled: true},
		{ProviderID: runnage.ID(), DestinationCountry: platformsms.CountryKenya, Priority: 1, Enabled: true},
	}}, routing.NewPriorityStrategy(), leamout, runnage)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	providers, err := router.Route(context.Background(), validKenyaRequest())
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if len(providers) != 2 {
		t.Fatalf("Route() count = %d, want 2", len(providers))
	}
	if providers[0].ID() != runnagesms.ProviderID || providers[1].ID() != leamoutsms.ProviderID {
		t.Fatalf("Route() order = %q, %q", providers[0].ID(), providers[1].ID())
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
	sender, err := platformsms.NewService(router)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	response, err := sender.Send(context.Background(), validKenyaRequest())
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
	sender, err := platformsms.NewService(router)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	response, err := sender.Send(context.Background(), validKenyaRequest())
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

func newTestRouter(
	t *testing.T,
	primary platformsms.Provider,
	secondary platformsms.Provider,
) *routing.Service {
	t.Helper()
	router, err := routing.NewService(routing.Config{Routes: []routing.Route{
		{ProviderID: primary.ID(), DestinationCountry: platformsms.CountryKenya, Priority: 1, Enabled: true},
		{ProviderID: secondary.ID(), DestinationCountry: platformsms.CountryKenya, Priority: 2, Enabled: true},
	}}, routing.NewPriorityStrategy(), primary, secondary)
	if err != nil {
		t.Fatalf("routing.NewService() error = %v", err)
	}
	return router
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
