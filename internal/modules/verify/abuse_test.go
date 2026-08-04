package verify

import (
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestDefaultAbusePolicyIsValid(t *testing.T) {
	t.Parallel()

	if err := validateAbusePolicy(DefaultAbusePolicy()); err != nil {
		t.Fatalf("validateAbusePolicy() error = %v", err)
	}
}

func TestValidateAbusePolicyRejectsDisabledLimit(t *testing.T) {
	t.Parallel()

	policy := DefaultAbusePolicy()
	policy.CreateRecipient.Count = 0
	if err := validateAbusePolicy(policy); err == nil {
		t.Fatal("validateAbusePolicy() accepted a zero limit")
	}
}

func TestRedisAbuseControlsRequireSecret(t *testing.T) {
	t.Parallel()

	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = client.Close() })
	if _, err := NewRedisAbuseControls(client, []byte("short"), DefaultAbusePolicy()); err == nil {
		t.Fatal("NewRedisAbuseControls() accepted a short secret")
	}
}

func TestAbuseDigestIsStableAndDoesNotExposeRecipient(t *testing.T) {
	t.Parallel()

	controls := &RedisAbuseControls{secret: []byte("0123456789abcdef0123456789abcdef")}
	first := controls.digest(" Team:Person@Example.com ")
	second := controls.digest("team:person@example.com")
	if first != second {
		t.Fatalf("digest normalization mismatch: %q != %q", first, second)
	}
	if strings.Contains(first, "person") || strings.Contains(first, "example") {
		t.Fatalf("digest exposes recipient: %q", first)
	}
	if len(first) != 64 {
		t.Fatalf("digest length = %d, want 64", len(first))
	}
}
