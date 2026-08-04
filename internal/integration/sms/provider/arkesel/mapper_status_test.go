package arkesel

import "testing"

func TestNormalizeStatusPreservesDeliveryMeaning(t *testing.T) {
	tests := map[string]string{
		"PENDING":   "submitted",
		"SENT":      "sent",
		"REJECTED":  "rejected",
		"DELIVERED": "delivered",
	}
	for input, want := range tests {
		if got := NormalizeStatus(input); got != want {
			t.Fatalf("NormalizeStatus(%q) = %q, want %q", input, got, want)
		}
	}
}
