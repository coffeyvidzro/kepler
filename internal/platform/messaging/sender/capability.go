package sender

import "fmt"

// Capability describes a provider feature required by messaging orchestration.
type Capability string

const (
	CapabilityDomainVerification   Capability = "domain_verification"
	CapabilitySenderIDRegistration Capability = "sender_id_registration"
	CapabilityPushDeliveryFeedback Capability = "push_delivery_feedback"
	CapabilityPollDeliveryFeedback Capability = "poll_delivery_feedback"
	CapabilityGeographicRouting    Capability = "geographic_routing"
)

func (capability Capability) Valid() bool {
	switch capability {
	case CapabilityDomainVerification,
		CapabilitySenderIDRegistration,
		CapabilityPushDeliveryFeedback,
		CapabilityPollDeliveryFeedback,
		CapabilityGeographicRouting:
		return true
	default:
		return false
	}
}

// CapabilitySet is the normalized set of features exposed by a provider route.
type CapabilitySet map[Capability]struct{}

// NewCapabilitySet validates and deduplicates provider capabilities.
func NewCapabilitySet(capabilities ...Capability) (CapabilitySet, error) {
	set := make(CapabilitySet, len(capabilities))
	for _, capability := range capabilities {
		if !capability.Valid() {
			return nil, fmt.Errorf("invalid sender provider capability %q", capability)
		}
		set[capability] = struct{}{}
	}
	return set, nil
}

// Supports reports whether the provider route exposes a capability.
func (set CapabilitySet) Supports(capability Capability) bool {
	if !capability.Valid() {
		return false
	}
	_, ok := set[capability]
	return ok
}

// SupportsAll reports whether the provider route exposes every required capability.
func (set CapabilitySet) SupportsAll(required ...Capability) bool {
	for _, capability := range required {
		if !set.Supports(capability) {
			return false
		}
	}
	return true
}

// Validate checks that every member of the set is a recognized capability.
func (set CapabilitySet) Validate() error {
	for capability := range set {
		if !capability.Valid() {
			return fmt.Errorf("invalid sender provider capability %q", capability)
		}
	}
	return nil
}
