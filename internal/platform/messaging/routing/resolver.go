package routing

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

type Repository interface {
	ListCandidates(context.Context, Request) ([]Candidate, error)
}

type Route struct {
	SenderAssetID           uuid.UUID
	SenderProviderBindingID uuid.UUID
	Provider                string
	ProviderAccount         string
	Region                  string
	CountryCode             string
}

type Resolver struct {
	repository Repository
	strategy   Strategy
}

func NewResolver(repository Repository, strategy Strategy) (*Resolver, error) {
	if repository == nil {
		return nil, errors.New("routing repository is required")
	}
	if strategy == nil {
		return nil, errors.New("routing strategy is required")
	}
	return &Resolver{repository: repository, strategy: strategy}, nil
}

func (resolver *Resolver) Resolve(ctx context.Context, request Request) (Route, error) {
	routes, err := resolver.ResolveAll(ctx, request)
	if err != nil {
		return Route{}, err
	}
	return routes[0], nil
}

func (resolver *Resolver) ResolveAll(ctx context.Context, request Request) ([]Route, error) {
	if resolver == nil || resolver.repository == nil || resolver.strategy == nil {
		return nil, errors.New("routing resolver is not configured")
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	candidates, err := resolver.repository.ListCandidates(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("list messaging route candidates: %w", err)
	}
	remaining := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if Evaluate(request, candidate).Eligible {
			remaining = append(remaining, candidate)
		}
	}
	if len(remaining) == 0 {
		return nil, ErrNoEligibleRoute
	}
	routes := make([]Route, 0, len(remaining))
	for len(remaining) > 0 {
		selected, selectErr := resolver.strategy.Select(request, remaining)
		if selectErr != nil {
			return nil, selectErr
		}
		selectedIndex := -1
		for index := range remaining {
			if remaining[index].Binding.ID == selected.Binding.ID {
				selectedIndex = index
				break
			}
		}
		if selectedIndex < 0 {
			return nil, errors.New("routing strategy selected an unknown candidate")
		}
		routes = append(routes, routeFromCandidate(selected))
		remaining = append(remaining[:selectedIndex], remaining[selectedIndex+1:]...)
	}
	return routes, nil
}

func routeFromCandidate(selected Candidate) Route {
	return Route{
		SenderAssetID:           selected.Asset.ID,
		SenderProviderBindingID: selected.Binding.ID,
		Provider:                selected.Binding.Provider,
		ProviderAccount:         selected.Binding.ProviderAccount,
		Region:                  selected.Binding.Region,
		CountryCode:             selected.Binding.CountryCode,
	}
}
