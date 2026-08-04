package webhookdelivery

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	platformwebhook "github.com/coffeyvidzro/dugble/server/internal/platform/webhook"
)

func TestSignUsesDisplayedSigningSecretAsKey(t *testing.T) {
	const secret = "whsec_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	const timestamp int64 = 1_700_000_000
	body := []byte(`{"event":"sms.delivered"}`)
	const want = "t=1700000000,v1=a5a2030c220be01d63f6a75eaa7dc3a5c291fec07d07bce95e5d6b29d526879f"

	if got := Sign([]byte(secret), timestamp, body); got != want {
		t.Fatalf("Sign() = %q, want %q", got, want)
	}
	if !strings.HasPrefix(secret, platformwebhook.SigningSecretPrefix) {
		t.Fatalf("NewSigningSecret() = %q, missing prefix", secret)
	}

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve webhook signing test path")
	}
	documentPath := filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "..", "docs", "webhooks", "signatures.mdx")
	document, err := os.ReadFile(documentPath)
	if err != nil {
		t.Fatalf("read webhook signature documentation: %v", err)
	}
	for _, fixtureValue := range []string{secret, `{"event":"sms.delivered"}`, want} {
		if !strings.Contains(string(document), fixtureValue) {
			t.Errorf("signature documentation does not contain test vector value %q", fixtureValue)
		}
	}
}
