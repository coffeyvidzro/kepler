package routing

import (
	"errors"
	"sort"
	"strings"

	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging/sender"
)

var ErrNoEligibleRoute = errors.New("no eligible messaging route is available")

// Strategy selects one candidate from a non-empty eligible set.
type Strategy interface {
	Select(Request, []Candidate) (Candidate, error)
}

// DeterministicStrategy prefers defaults and exact geography, then uses stable
// provider and binding identifiers to make route selection repeatable.
type DeterministicStrategy struct{}

func (DeterministicStrategy) Select(request Request, candidates []Candidate) (Candidate, error) {
	if len(candidates) == 0 {
		return Candidate{}, ErrNoEligibleRoute
	}

	ranked := append([]Candidate(nil), candidates...)
	sort.SliceStable(ranked, func(leftIndex, rightIndex int) bool {
		left := ranked[leftIndex]
		right := ranked[rightIndex]

		if left.Grant.Default != right.Grant.Default {
			return left.Grant.Default
		}
		leftCountry := exact(request.DestinationCountry, left.Binding.CountryCode)
		rightCountry := exact(request.DestinationCountry, right.Binding.CountryCode)
		if leftCountry != rightCountry {
			return leftCountry
		}
		leftRegion := exact(request.DestinationRegion, left.Binding.Region)
		rightRegion := exact(request.DestinationRegion, right.Binding.Region)
		if leftRegion != rightRegion {
			return leftRegion
		}
		leftHealthy := left.Binding.HealthStatus == sender.HealthHealthy
		rightHealthy := right.Binding.HealthStatus == sender.HealthHealthy
		if leftHealthy != rightHealthy {
			return leftHealthy
		}
		if left.Binding.Provider != right.Binding.Provider {
			return left.Binding.Provider < right.Binding.Provider
		}
		if left.Binding.ProviderAccount != right.Binding.ProviderAccount {
			return left.Binding.ProviderAccount < right.Binding.ProviderAccount
		}
		return left.Binding.ID.String() < right.Binding.ID.String()
	})
	return ranked[0], nil
}

func exact(requested, available string) bool {
	requested = strings.TrimSpace(requested)
	available = strings.TrimSpace(available)
	return requested != "" && available != "" && strings.EqualFold(requested, available)
}
