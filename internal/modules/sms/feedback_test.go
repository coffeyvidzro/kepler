package sms

import "testing"

func TestNextSMSFeedbackStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current string
		next    string
		want    string
	}{
		{name: "advance", current: StatusSubmitted, next: StatusSent, want: StatusSent},
		{name: "terminal", current: StatusSent, next: StatusDelivered, want: StatusDelivered},
		{name: "no regression", current: StatusSent, next: StatusSubmitted, want: StatusSent},
		{name: "unknown", current: StatusSent, next: StatusUnknown, want: StatusUnknown},
		{name: "recover unknown", current: StatusUnknown, next: StatusDelivered, want: StatusDelivered},
		{name: "terminal stays terminal", current: StatusDelivered, next: StatusFailed, want: StatusDelivered},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := nextSMSFeedbackStatus(test.current, test.next); got != test.want {
				t.Fatalf("nextSMSFeedbackStatus(%q, %q) = %q, want %q", test.current, test.next, got, test.want)
			}
		})
	}
}
