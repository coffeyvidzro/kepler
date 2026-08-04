package ses

import (
	"testing"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"
)

func TestConfigurationSetARN(t *testing.T) {
	got, err := configurationSetARN(
		"arn:aws:ses:eu-north-1:123456789012:tenant/dugble-t-example/tn-123",
		TransactionalConfigurationSet,
	)
	if err != nil {
		t.Fatalf("configurationSetARN() error = %v", err)
	}
	want := "arn:aws:ses:eu-north-1:123456789012:configuration-set/dugble-transactional"
	if got != want {
		t.Fatalf("configurationSetARN() = %q, want %q", got, want)
	}
}

func TestIdentityARN(t *testing.T) {
	got, err := identityARN(
		"arn:aws:ses:eu-north-1:123456789012:tenant/dugble-t-example/tn-123",
		"Example.COM",
	)
	if err != nil {
		t.Fatalf("identityARN() error = %v", err)
	}
	want := "arn:aws:ses:eu-north-1:123456789012:identity/example.com"
	if got != want {
		t.Fatalf("identityARN() = %q, want %q", got, want)
	}
}

func TestOnboardingIdentityARN(t *testing.T) {
	got, err := identityARN(
		"arn:aws:ses:eu-north-1:123456789012:tenant/dugble-t-example/tn-123",
		platformemail.CustomerOnboardingSESIdentity,
	)
	if err != nil {
		t.Fatalf("identityARN() error = %v", err)
	}
	want := "arn:aws:ses:eu-north-1:123456789012:identity/runnage.dev"
	if got != want {
		t.Fatalf("identityARN() = %q, want %q", got, want)
	}
}

func TestReputationPolicyARN(t *testing.T) {
	got, err := reputationPolicyARN(
		"arn:aws:ses:eu-north-1:123456789012:tenant/dugble-t-example/tn-123",
		"strict",
	)
	if err != nil {
		t.Fatalf("reputationPolicyARN() error = %v", err)
	}
	want := "arn:aws:ses:eu-north-1:aws:reputation-policy/strict"
	if got != want {
		t.Fatalf("reputationPolicyARN() = %q, want %q", got, want)
	}
}

func TestReputationPolicyARNRejectsUnknownPolicy(t *testing.T) {
	_, err := reputationPolicyARN(
		"arn:aws:ses:eu-north-1:123456789012:tenant/dugble-t-example/tn-123",
		"aggressive",
	)
	if err == nil {
		t.Fatal("reputationPolicyARN() expected error")
	}
}
