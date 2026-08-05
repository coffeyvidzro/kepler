package sms

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/coffeyvidzro/dugble/server/internal/adapters/moolre"
	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

func TestProviderSendUsesDugbleReference(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != sendPath {
			t.Fatalf("path = %s, want %s", request.URL.Path, sendPath)
		}
		var payload sendRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.Type != sendType || payload.SenderID != "Dugble" || len(payload.Messages) != 1 {
			t.Fatalf("payload = %#v", payload)
		}
		message := payload.Messages[0]
		if message.Recipient != "233201234567" || message.Reference != "message-uuid" {
			t.Fatalf("message = %#v", message)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"status":1,"code":"SMS01","message":"Success","data":null,"go":null}`))
	}))
	defer server.Close()

	provider := NewProvider(moolre.NewClientWithHTTP("vas-key", testHTTPClient(t, server)))
	result, err := provider.Send(context.Background(), platformsms.SendRequest{
		Reference:          "message-uuid",
		To:                 "+233201234567",
		From:               "Dugble",
		Message:            "Hello",
		DestinationCountry: "GH",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if result.ProviderID != ProviderID || result.ProviderMsgID != "message-uuid" || result.Status != platformsms.StatusSubmitted {
		t.Fatalf("result = %#v", result)
	}
}

func TestProviderCheckStatusPreservesUndocumentedNumericStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != statusPath {
			t.Fatalf("path = %s, want %s", request.URL.Path, statusPath)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"status":1,"code":"ASMQ10","message":"SMS Status","data":[{"ref":"message-uuid","status":3}],"go":null}`))
	}))
	defer server.Close()

	provider := NewProvider(moolre.NewClientWithHTTP("vas-key", testHTTPClient(t, server)))
	result, err := provider.CheckStatus(context.Background(), "message-uuid")
	if err != nil {
		t.Fatalf("CheckStatus() error = %v", err)
	}
	if result.Status != platformsms.StatusUnknown || result.ProviderStatus != "3" {
		t.Fatalf("result = %#v", result)
	}
}

func testHTTPClient(t *testing.T, server *httptest.Server) *http.Client {
	t.Helper()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	transport := server.Client().Transport
	return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		clone := request.Clone(request.Context())
		clone.URL.Scheme = target.Scheme
		clone.URL.Host = target.Host
		return transport.RoundTrip(clone)
	})}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
