package httpclient

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestFixedEndpointClientAllowsSameOriginHTTPSRedirect(t *testing.T) {
	client := ForFixedEndpoint("https://api.example.com", nil, time.Second)
	request := &http.Request{URL: mustParseURL(t, "https://api.example.com/v2")}

	if err := client.CheckRedirect(request, []*http.Request{{}}); err != nil {
		t.Fatalf("CheckRedirect() error = %v", err)
	}
}

func TestFixedEndpointClientRejectsPlainHTTPRedirect(t *testing.T) {
	client := ForFixedEndpoint("https://api.example.com", nil, time.Second)
	request := &http.Request{URL: mustParseURL(t, "http://api.example.com/v2")}

	if err := client.CheckRedirect(request, []*http.Request{{}}); err == nil {
		t.Fatal("CheckRedirect() error = nil")
	}
}

func TestFixedEndpointClientRejectsDifferentHostRedirect(t *testing.T) {
	client := ForFixedEndpoint("https://api.example.com", nil, time.Second)
	request := &http.Request{URL: mustParseURL(t, "https://attacker.example/v2")}

	if err := client.CheckRedirect(request, []*http.Request{{}}); err == nil {
		t.Fatal("CheckRedirect() error = nil")
	}
}

func TestFixedEndpointClientOverridesInjectedRedirectPolicy(t *testing.T) {
	injected := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return nil }}
	client := ForFixedEndpoint("https://api.example.com", injected, time.Second)
	request := &http.Request{URL: mustParseURL(t, "http://api.example.com/v2")}

	if err := client.CheckRedirect(request, []*http.Request{{}}); err == nil {
		t.Fatal("CheckRedirect() error = nil")
	}
}

func mustParseURL(t *testing.T, value string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	return parsed
}
