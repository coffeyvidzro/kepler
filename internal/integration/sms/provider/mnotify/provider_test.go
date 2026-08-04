package mnotify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coffeyvidzro/dugble/server/internal/integration/sms"
)

func TestProviderSendUsesQuickEndpoint(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/sms/quick" {
			t.Errorf("path = %s, want /api/sms/quick", r.URL.Path)
		}
		if r.URL.Query().Get("key") != "secret" {
			t.Errorf("key = %q, want secret", r.URL.Query().Get("key"))
		}

		var request SendRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len(request.Recipient) != 1 || request.Recipient[0] != "233201234567" {
			t.Errorf("recipient = %#v, want [233201234567]", request.Recipient)
		}
		if request.IsSchedule || request.ScheduleDate != "" {
			t.Errorf("unexpected provider scheduling fields: %#v", request)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status":"success",
			"code":2000,
			"message":"messages sent successfully",
			"summary":{"_id":"campaign-1","total_sent":1,"contacts":1,"total_rejected":0}
		}`))
	}))
	defer server.Close()

	provider := NewProvider(&Client{
		BaseURL:    server.URL,
		APIKey:     "secret",
		HTTPClient: server.Client(),
	})
	response, err := provider.Send(context.Background(), sms.SendRequest{
		To:      "+233201234567",
		From:    "DUGBLE",
		Message: "hello",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if response.ProviderID != providerID {
		t.Fatalf("ProviderID = %q, want %q", response.ProviderID, providerID)
	}
	if response.ProviderMsgID != "campaign-1" {
		t.Fatalf("ProviderMsgID = %q, want campaign-1", response.ProviderMsgID)
	}
	if response.Status != sms.StatusSubmitted {
		t.Fatalf("Status = %q, want %q", response.Status, sms.StatusSubmitted)
	}
}

func TestProviderCheckStatusUsesCampaignEndpoint(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/campaign/campaign-1" {
			t.Errorf("path = %s, want /api/campaign/campaign-1", r.URL.Path)
		}
		if r.URL.Query().Get("key") != "secret" {
			t.Errorf("key = %q, want secret", r.URL.Query().Get("key"))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status":"success",
			"report":[{
				"_id":1,
				"recipient":"233201234567",
				"status":"DELIVERED",
				"campaign_id":"campaign-1",
				"retries":0
			}]
		}`))
	}))
	defer server.Close()

	provider := NewProvider(&Client{
		BaseURL:    server.URL,
		APIKey:     "secret",
		HTTPClient: server.Client(),
	})
	response, err := provider.CheckStatus(context.Background(), "campaign-1")
	if err != nil {
		t.Fatalf("CheckStatus() error = %v", err)
	}
	if response.ProviderMsgID != "campaign-1" {
		t.Fatalf("ProviderMsgID = %q, want campaign-1", response.ProviderMsgID)
	}
	if response.Status != sms.StatusDelivered {
		t.Fatalf("Status = %q, want %q", response.Status, sms.StatusDelivered)
	}
}
