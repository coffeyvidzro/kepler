package moolre

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClientUsesProductionEndpoint(t *testing.T) {
	client := NewClient("vas-secret")
	if client.baseURL != productionBaseURL {
		t.Fatalf("base URL = %q, want %q", client.baseURL, productionBaseURL)
	}
}

func TestClientPostSetsVASKeyAndDecodesEnvelope(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", request.Method)
		}
		if request.URL.Path != "/open/sms/send" {
			t.Fatalf("path = %s, want /open/sms/send", request.URL.Path)
		}
		if value := request.Header.Get("X-API-VASKEY"); value != "vas-secret" {
			t.Fatalf("X-API-VASKEY = %q, want vas-secret", value)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"status":1,"code":"SMS01","message":"Success","data":null,"go":null}`))
	}))
	defer server.Close()

	client := newClient(server.URL, "vas-secret", server.Client())
	var result Envelope[any]
	if err := client.Post(context.Background(), "/open/sms/send", map[string]int{"type": 1}, &result); err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	if !result.Successful() || result.Code != "SMS01" {
		t.Fatalf("result = %#v", result)
	}
}

func TestClientPostReturnsStructuredHTTPError(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusUnauthorized)
		_, _ = response.Write([]byte(`{"status":0,"code":"AIN01","message":"Authentication Error","data":null,"go":null}`))
	}))
	defer server.Close()

	client := newClient(server.URL, "invalid", server.Client())
	err := client.Post(context.Background(), "/open/sms/send", map[string]int{"type": 1}, &Envelope[any]{})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want *APIError", err)
	}
	if apiErr.Code != "AIN01" || apiErr.Message != "Authentication Error" {
		t.Fatalf("API error = %#v", apiErr)
	}
	if !apiErr.SafeToFallback() {
		t.Fatal("authentication error should be definitive")
	}
}
