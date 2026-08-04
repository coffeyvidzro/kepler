package event

import (
	"reflect"
	"testing"
)

func TestSubscribableTypesIncludesVerifyCatalog(t *testing.T) {
	want := []string{
		"sms.submitted",
		"sms.sent",
		"sms.delivered",
		"sms.undelivered",
		"sms.failed",
		"email.submitted",
		"email.delivered",
		"email.delayed",
		"email.bounced",
		"email.complained",
		"email.rejected",
		"email.failed",
		"email.opened",
		"email.clicked",
		"email.subscription_changed",
		"verification.created",
		"verification.dispatched",
		"verification.approved",
		"verification.incorrect",
		"verification.resent",
		"verification.expired",
		"verification.delivery_failed",
		"verification.max_attempts_reached",
		"verification.canceled",
	}
	if got := SubscribableTypes(); !reflect.DeepEqual(got, want) {
		t.Fatalf("SubscribableTypes() = %v, want %v", got, want)
	}
}

func TestDefinitionsReturnsCopy(t *testing.T) {
	catalog := Definitions()
	catalog[0].ObjectType = "modified"
	definition, ok := Lookup(TypeSMSSubmitted)
	if !ok {
		t.Fatal("Lookup(TypeSMSSubmitted) did not find event")
	}
	if definition.ObjectType != "sms" {
		t.Fatalf("Lookup(TypeSMSSubmitted).ObjectType = %q, want sms", definition.ObjectType)
	}
}

func TestVerificationEventsAreSubscribable(t *testing.T) {
	definition, ok := Lookup(TypeVerificationCreated)
	if !ok {
		t.Fatal("Lookup(TypeVerificationCreated) did not find event")
	}
	if definition.ObjectType != "verification" || !definition.ObjectIDRequired {
		t.Fatalf("verification definition = %+v", definition)
	}
	if !IsSubscribable(TypeVerificationCreated) {
		t.Fatal("verification.created should be subscribable after the Verify API is activated")
	}
}

func TestParseTypeTrimsAndRejectsUnknownValues(t *testing.T) {
	got, err := ParseType("  sms.sent  ")
	if err != nil {
		t.Fatalf("ParseType() error = %v", err)
	}
	if got != TypeSMSSent {
		t.Fatalf("ParseType() = %q, want %q", got, TypeSMSSent)
	}
	if _, err := ParseType("unknown.event"); err == nil {
		t.Fatal("ParseType() accepted an unknown event type")
	}
}
