package mnotify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClientUsesProductionEndpoint(t *testing.T) {
	client := NewClient("api-key")
	if client.baseURL != productionBaseURL {
		t.Fatalf("base URL = %q, want %q", client.baseURL, productionBaseURL)
	}
}

func TestClientPostsWithAPIKey(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("method = %q", request.Method)
		}
		if request.URL.Path != "/api/test" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		if request.URL.Query().Get("key") != "api-key" {
			t.Fatalf("key = %q", request.URL.Query().Get("key"))
		}
		if request.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("content type = %q", request.Header.Get("Content-Type"))
		}
		var payload map[string]string
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["value"] != "test" {
			t.Fatalf("payload = %#v", payload)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"status":"success","code":2000,"message":"ok"}`))
	}))
	defer server.Close()

	client := newClient(server.URL, "api-key", server.Client())
	var result Response
	if err := client.Post(context.Background(), "/api/test", map[string]string{"value": "test"}, &result); err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	if !result.Successful() || result.Code.String() != "2000" {
		t.Fatalf("result = %#v", result)
	}
}

func TestClientReturnsStructuredHTTPError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusUnauthorized)
		_, _ = response.Write([]byte(`{"status":"error","code":4010,"message":"invalid key"}`))
	}))
	defer server.Close()

	client := newClient(server.URL, "api-key", server.Client())
	err := client.Get(context.Background(), "/api/test", &Response{})
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized || apiErr.Code.String() != "4010" {
		t.Fatalf("error = %#v", apiErr)
	}
}
