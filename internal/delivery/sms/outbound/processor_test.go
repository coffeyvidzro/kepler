package smsdelivery

import "testing"

func TestProviderAvailable(t *testing.T) {
	t.Parallel()

	providers := []string{"mnotify", " moolre "}
	for _, provider := range []string{"MNOTIFY", "moolre"} {
		if !providerAvailable(provider, providers) {
			t.Fatalf("providerAvailable(%q) = false, want true", provider)
		}
	}
	for _, provider := range []string{"", "unknown"} {
		if providerAvailable(provider, providers) {
			t.Fatalf("providerAvailable(%q) = true, want false", provider)
		}
	}
}
