package httptransport

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIPExtractorIgnoresForwardedHeaderWithoutTrustedProxy(t *testing.T) {
	extractor, err := newClientIPExtractor([]string{})
	if err != nil {
		t.Fatalf("create client IP extractor: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "192.0.2.10:43123"
	request.Header.Set("X-Forwarded-For", "198.51.100.7")

	if got := extractor(request); got != "192.0.2.10" {
		t.Fatalf("client IP = %q, want direct peer", got)
	}
}

func TestClientIPExtractorIgnoresForwardedHeaderFromUntrustedPeer(t *testing.T) {
	extractor, err := newClientIPExtractor([]string{"10.10.0.0/24"})
	if err != nil {
		t.Fatalf("create client IP extractor: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "192.0.2.10:43123"
	request.Header.Set("X-Forwarded-For", "198.51.100.7")

	if got := extractor(request); got != "192.0.2.10" {
		t.Fatalf("client IP = %q, want untrusted direct peer", got)
	}
}

func TestClientIPExtractorRejectsSpoofedLeftmostForwardedAddress(t *testing.T) {
	extractor, err := newClientIPExtractor([]string{"10.10.0.0/24"})
	if err != nil {
		t.Fatalf("create client IP extractor: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "10.10.0.2:443"
	request.Header.Set("X-Forwarded-For", "198.51.100.7, 203.0.113.9")

	if got := extractor(request); got != "203.0.113.9" {
		t.Fatalf("client IP = %q, want nearest untrusted address", got)
	}
}

func TestClientIPExtractorWalksTrustedProxyChain(t *testing.T) {
	extractor, err := newClientIPExtractor([]string{"10.10.0.0/24"})
	if err != nil {
		t.Fatalf("create client IP extractor: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "10.10.0.2:443"
	request.Header.Set("X-Forwarded-For", "198.51.100.7, 10.10.0.3")

	if got := extractor(request); got != "198.51.100.7" {
		t.Fatalf("client IP = %q, want original client after trusted chain", got)
	}
}

func TestClientIPExtractorLoadsTrustedCIDRsFromEnvironment(t *testing.T) {
	t.Setenv(trustedProxyCIDRsEnv, "10.10.0.0/24")
	extractor, err := newClientIPExtractor(nil)
	if err != nil {
		t.Fatalf("create client IP extractor: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "10.10.0.2:443"
	request.Header.Set("X-Forwarded-For", "198.51.100.7")

	if got := extractor(request); got != "198.51.100.7" {
		t.Fatalf("client IP = %q, want forwarded client", got)
	}
}

func TestClientIPExtractorRejectsInvalidTrustedCIDR(t *testing.T) {
	if _, err := newClientIPExtractor([]string{"not-a-cidr"}); err == nil {
		t.Fatal("expected invalid trusted proxy CIDR to fail")
	}
}
