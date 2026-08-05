package routing

import (
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging"
	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging/sender"
)

// Request describes the tenant, channel and destination requirements for route selection.
type Request struct {
	TeamID               uuid.UUID
	Channel              messaging.Channel
	DestinationCountry   string
	DestinationRegion    string
	RequiredCapabilities []sender.Capability
}

func (request Request) Validate() error {
	if request.TeamID == uuid.Nil {
		return errors.New("routing team ID is required")
	}
	if !request.Channel.Valid() {
		return errors.New("routing channel is invalid")
	}
	country := strings.TrimSpace(request.DestinationCountry)
	if country != "" && !validCountryCode(country) {
		return errors.New("routing destination country is invalid")
	}
	for _, capability := range request.RequiredCapabilities {
		if !capability.Valid() {
			return errors.New("routing capability is invalid")
		}
	}
	return nil
}

// Candidate combines the canonical sender objects needed to evaluate one route.
type Candidate struct {
	Asset        sender.Asset
	Grant        sender.Grant
	Binding      sender.ProviderBinding
	Capabilities sender.CapabilitySet
}

// RejectionCode is a stable machine-readable routing rejection reason.
type RejectionCode string

const (
	RejectionInvalidCandidate      RejectionCode = "invalid_candidate"
	RejectionAssetUnavailable      RejectionCode = "asset_unavailable"
	RejectionAssetUnhealthy        RejectionCode = "asset_unhealthy"
	RejectionGrantUnavailable      RejectionCode = "grant_unavailable"
	RejectionBindingUnavailable    RejectionCode = "binding_unavailable"
	RejectionBindingUnverified     RejectionCode = "binding_unverified"
	RejectionBindingUnhealthy      RejectionCode = "binding_unhealthy"
	RejectionCountryMismatch       RejectionCode = "country_mismatch"
	RejectionRegionMismatch        RejectionCode = "region_mismatch"
	RejectionCapabilityUnavailable RejectionCode = "capability_unavailable"
)

// Rejection explains why a candidate cannot satisfy a route request.
type Rejection struct {
	Code       RejectionCode
	Capability sender.Capability
}

// Evaluation contains the complete eligibility result for one candidate.
type Evaluation struct {
	Eligible   bool
	Rejections []Rejection
}

// Evaluate applies tenant, lifecycle, health, geography and capability rules.
func Evaluate(request Request, candidate Candidate) Evaluation {
	var rejections []Rejection
	asset := candidate.Asset
	grant := candidate.Grant
	binding := candidate.Binding

	if asset.ID == uuid.Nil || grant.ID == uuid.Nil || binding.ID == uuid.Nil ||
		grant.SenderAssetID != asset.ID || binding.SenderAssetID != asset.ID ||
		grant.TeamID != request.TeamID || grant.Channel != request.Channel ||
		asset.Channel != request.Channel {
		rejections = append(rejections, Rejection{Code: RejectionInvalidCandidate})
	}
	if asset.Status != sender.AssetStatusActive {
		rejections = append(rejections, Rejection{Code: RejectionAssetUnavailable})
	}
	if asset.HealthStatus == sender.HealthDegraded {
		rejections = append(rejections, Rejection{Code: RejectionAssetUnhealthy})
	}
	if grant.Status != sender.GrantStatusActive || grant.RevokedAt != nil {
		rejections = append(rejections, Rejection{Code: RejectionGrantUnavailable})
	}
	if binding.Status != sender.BindingStatusActive {
		rejections = append(rejections, Rejection{Code: RejectionBindingUnavailable})
	}
	if !binding.Verified {
		rejections = append(rejections, Rejection{Code: RejectionBindingUnverified})
	}
	if binding.HealthStatus == sender.HealthDegraded {
		rejections = append(rejections, Rejection{Code: RejectionBindingUnhealthy})
	}
	if request.DestinationCountry != "" && binding.CountryCode != "" &&
		!strings.EqualFold(request.DestinationCountry, binding.CountryCode) {
		rejections = append(rejections, Rejection{Code: RejectionCountryMismatch})
	}
	if request.DestinationRegion != "" && binding.Region != "" &&
		!strings.EqualFold(request.DestinationRegion, binding.Region) {
		rejections = append(rejections, Rejection{Code: RejectionRegionMismatch})
	}
	for _, capability := range request.RequiredCapabilities {
		if !candidate.Capabilities.Supports(capability) {
			rejections = append(rejections, Rejection{
				Code:       RejectionCapabilityUnavailable,
				Capability: capability,
			})
		}
	}

	return Evaluation{Eligible: len(rejections) == 0, Rejections: rejections}
}

func validCountryCode(value string) bool {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) != 2 {
		return false
	}
	for _, character := range value {
		if character < 'A' || character > 'Z' {
			return false
		}
	}
	return true
}
