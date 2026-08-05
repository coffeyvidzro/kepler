package sender

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coffeyvidzro/dugble/server/internal/adapters/mnotify"
	platformsenderid "github.com/coffeyvidzro/dugble/server/internal/platform/senderid"
)

func TestProviderCreatesAndChecksSenderID(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("key") != "api-key" {
			t.Fatalf("key = %q", request.URL.Query().Get("key"))
		}
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case createPath:
			var payload createRequest
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatalf("decode create request: %v", err)
			}
			if payload.SenderName != "Dugble1" || payload.Purpose != "Transactional alerts" {
				t.Fatalf("create payload = %#v", payload)
			}
			_, _ = response.Write([]byte(`{"status":"success","code":2000,"message":"sender ID submitted","summary":{"sender_name":"Dugble1","purpose":"Transactional alerts","status":"On Hold"}}`))
		case statusPath:
			var payload statusRequest
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatalf("decode status request: %v", err)
			}
			if payload.SenderName != "Dugble1" {
				t.Fatalf("status payload = %#v", payload)
			}
			_, _ = response.Write([]byte(`{"status":"success","code":2000,"message":"sender ID status","summary":{"sender name":"Dugble1","status":"Approved"}}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	provider := NewProvider(mnotify.NewClientWithHTTP(server.URL, "api-key", server.Client()))
	created, err := provider.Create(context.Background(), platformsenderid.CreateRequest{
		SenderID: "Dugble1",
		Purpose:  "Transactional alerts",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ProviderID != ProviderID || created.SenderID != "Dugble1" || created.Status != platformsenderid.StatusPending {
		t.Fatalf("created = %#v", created)
	}

	status, err := provider.CheckStatus(context.Background(), "Dugble1")
	if err != nil {
		t.Fatalf("CheckStatus() error = %v", err)
	}
	if status.Status != platformsenderid.StatusApproved || status.ProviderStatus != "Approved" {
		t.Fatalf("status = %#v", status)
	}
}

func TestProviderRequiresPurpose(t *testing.T) {
	t.Parallel()

	provider := NewProvider(mnotify.NewClient("https://api.mnotify.com", "api-key"))
	_, err := provider.Create(context.Background(), platformsenderid.CreateRequest{SenderID: "Dugble1"})
	if err == nil {
		t.Fatal("Create() error = nil")
	}
}
