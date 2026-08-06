package routing

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging"
	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging/sender"
)

type repositoryStub struct {
	candidates []Candidate
	err        error
}

func (repository repositoryStub) ListCandidates(context.Context, Request) ([]Candidate, error) {
	return repository.candidates, repository.err
}

func TestResolverSelectsEligibleDefaultRoute(t *testing.T) {
	t.Parallel()

	teamID := uuid.New()
	first := candidate(teamID, "zeta", false, true, "GH")
	second := candidate(teamID, "alpha", true, true, "GH")
	resolver, err := NewResolver(repositoryStub{candidates: []Candidate{first, second}}, DeterministicStrategy{})
	if err != nil {
		t.Fatalf("NewResolver() error = %v", err)
	}

	route, err := resolver.Resolve(context.Background(), Request{
		TeamID:             teamID,
		Channel:            messaging.ChannelSMS,
		DestinationCountry: "GH",
		RequiredCapabilities: []sender.Capability{
			sender.CapabilitySenderIDRegistration,
		},
	})
	if err != nil {
		t.Fatalf("Resolver.Resolve() error = %v", err)
	}
	if route.SenderProviderBindingID != second.Binding.ID {
		t.Fatalf("Resolver.Resolve() selected binding = %s, want %s", route.SenderProviderBindingID, second.Binding.ID)
	}
}

func TestResolverRejectsUnverifiedBinding(t *testing.T) {
	t.Parallel()

	teamID := uuid.New()
	unverified := candidate(teamID, "alpha", true, false, "GH")
	resolver, err := NewResolver(repositoryStub{candidates: []Candidate{unverified}}, DeterministicStrategy{})
	if err != nil {
		t.Fatalf("NewResolver() error = %v", err)
	}

	_, err = resolver.Resolve(context.Background(), Request{
		TeamID:             teamID,
		Channel:            messaging.ChannelSMS,
		DestinationCountry: "GH",
	})
	if !errors.Is(err, ErrNoEligibleRoute) {
		t.Fatalf("Resolver.Resolve() error = %v, want ErrNoEligibleRoute", err)
	}
}

func candidate(teamID uuid.UUID, provider string, defaultGrant, verified bool, country string) Candidate {
	assetID := uuid.New()
	capabilities, _ := sender.NewCapabilitySet(sender.CapabilitySenderIDRegistration)
	return Candidate{
		Asset: sender.Asset{
			ID:           assetID,
			Channel:      messaging.ChannelSMS,
			Status:       sender.AssetStatusActive,
			HealthStatus: sender.HealthHealthy,
		},
		Grant: sender.Grant{
			ID:            uuid.New(),
			TeamID:        teamID,
			SenderAssetID: assetID,
			Channel:       messaging.ChannelSMS,
			Status:        sender.GrantStatusActive,
			Default:       defaultGrant,
		},
		Binding: sender.ProviderBinding{
			ID:              uuid.New(),
			SenderAssetID:   assetID,
			Provider:        provider,
			ProviderAccount: "default",
			CountryCode:     country,
			Status:          sender.BindingStatusActive,
			Verified:        verified,
			HealthStatus:    sender.HealthHealthy,
		},
		Capabilities: capabilities,
	}
}

func TestResolverResolveAllRestrictsSenderAsset(t *testing.T) {
	t.Parallel()

	teamID := uuid.New()
	selected := candidate(teamID, "alpha", false, true, "GH")
	other := candidate(teamID, "beta", true, true, "GH")
	assetID := selected.Asset.ID
	resolver, err := NewResolver(repositoryStub{candidates: []Candidate{other, selected}}, DeterministicStrategy{})
	if err != nil {
		t.Fatalf("NewResolver() error = %v", err)
	}

	routes, err := resolver.ResolveAll(context.Background(), Request{
		TeamID:             teamID,
		Channel:            messaging.ChannelSMS,
		SenderAssetID:      &assetID,
		DestinationCountry: "GH",
	})
	if err != nil {
		t.Fatalf("Resolver.ResolveAll() error = %v", err)
	}
	if len(routes) != 1 || routes[0].SenderAssetID != assetID {
		t.Fatalf("Resolver.ResolveAll() routes = %#v, want only asset %s", routes, assetID)
	}
}

func TestResolverHonorsProviderAndAccountConstraints(t *testing.T) {
	t.Parallel()

	teamID := uuid.New()
	first := candidate(teamID, "alpha", true, true, "GH")
	second := candidate(teamID, "zeta", false, true, "GH")
	resolver, err := NewResolver(repositoryStub{candidates: []Candidate{first, second}}, DeterministicStrategy{})
	if err != nil {
		t.Fatalf("NewResolver() error = %v", err)
	}

	route, err := resolver.Resolve(context.Background(), Request{
		TeamID:          teamID,
		Channel:         messaging.ChannelSMS,
		Provider:        "zeta",
		ProviderAccount: "default",
	})
	if err != nil {
		t.Fatalf("Resolver.Resolve() error = %v", err)
	}
	if route.SenderProviderBindingID != second.Binding.ID {
		t.Fatalf("Resolver.Resolve() selected binding = %s, want %s", route.SenderProviderBindingID, second.Binding.ID)
	}
}
