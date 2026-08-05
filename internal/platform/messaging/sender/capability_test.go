package sender

import "testing"

func TestCapabilitySet(t *testing.T) {
	t.Parallel()

	set, err := NewCapabilitySet(
		CapabilitySenderIDRegistration,
		CapabilityPollDeliveryFeedback,
		CapabilitySenderIDRegistration,
	)
	if err != nil {
		t.Fatalf("NewCapabilitySet() error = %v", err)
	}
	if !set.SupportsAll(CapabilitySenderIDRegistration, CapabilityPollDeliveryFeedback) {
		t.Fatal("CapabilitySet.SupportsAll() = false for configured capabilities")
	}
	if set.Supports(CapabilityDomainVerification) {
		t.Fatal("CapabilitySet.Supports() = true for missing capability")
	}
	if _, err := NewCapabilitySet(Capability("invalid")); err == nil {
		t.Fatal("NewCapabilitySet() error = nil for invalid capability")
	}
}
