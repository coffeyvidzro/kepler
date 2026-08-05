package sender

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/coffeyvidzro/dugble/server/internal/adapters/moolre"
)

func TestProviderCreatesAndChecksSenderID(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case createPath:
			var payload createRequest
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatalf("decode create request: %v", err)
			}
			if payload.Type != createType || len(payload.SenderIDs) != 1 || payload.SenderIDs[0].SenderID != "Dugble" {
				t.Fatalf("create payload = %#v", payload)
			}
			_, _ = response.Write([]byte(`{"status":1,"code":"ASMQ12","message":"Sender IDs Created Successfully.","data":null,"go":null}`))
		case statusPath:
			var payload statusRequest
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatalf("decode status request: %v", err)
			}
			if payload.Type != statusType || payload.SenderID != "Dugble" {
				t.Fatalf("status payload = %#v", payload)
			}
			_, _ = response.Write([]byte(`{"status":1,"code":"ASMQ01","message":"Sender ID Status","data":{"senderid":"Dugble","approval":"Approved","whitelisted":false},"go":null}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	provider := NewProvider(moolre.NewClientWithHTTP("vas-key", testHTTPClient(t, server)))
	created, err := provider.Create(context.Background(), CreateRequest{SenderID: "Dugble"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ProviderID != ProviderID || created.Status != StatusPending {
		t.Fatalf("created = %#v", created)
	}

	status, err := provider.CheckStatus(context.Background(), "Dugble")
	if err != nil {
		t.Fatalf("CheckStatus() error = %v", err)
	}
	if status.Status != StatusApproved || status.ProviderStatus != "Approved" {
		t.Fatalf("status = %#v", status)
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
