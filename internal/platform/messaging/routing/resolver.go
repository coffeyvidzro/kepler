package routing

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Repository loads sender assets, grants, bindings and provider capabilities
// without exposing SQLC or database-specific types to the domain package.
type Repository interface {
	ListCandidates(context.Context, Request) ([]Candidate, error)
}

// Route is the selected provider and canonical sender binding.
type Route struct {
	SenderAssetID           uuid.UUID
	SenderProviderBindingID uuid.UUID
	Provider                string
	ProviderAccount         string
	Region                  string
	CountryCode             string
}

// Resolver filters provider routes through eligibility rules before applying a strategy.
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
	if resolver == nil || resolver.repository == nil || resolver.strategy == nil {
		return Route{}, errors.New("routing resolver is not configured")
	}
	if err := request.Validate(); err != nil {
		return Route{}, err
	}
	candidates, err := resolver.repository.ListCandidates(ctx, request)
	if err != nil {
		return Route{}, fmt.Errorf("list messaging route candidates: %w", err)
	}

	eligible := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if Evaluate(request, candidate).Eligible {
			eligible = append(eligible, candidate)
		}
	}
	selected, err := resolver.strategy.Select(request, eligible)
	if err != nil {
		return Route{}, err
	}
	return Route{
		SenderAssetID:           selected.Asset.ID,
		SenderProviderBindingID: selected.Binding.ID,
		Provider:                selected.Binding.Provider,
		ProviderAccount:         selected.Binding.ProviderAccount,
		Region:                  selected.Binding.Region,
		CountryCode:             selected.Binding.CountryCode,
	}, nil
}
