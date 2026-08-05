package sender

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	provider := NewProvider(moolre.NewClientWithHTTP(server.URL, "vas-key", server.Client()))
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
