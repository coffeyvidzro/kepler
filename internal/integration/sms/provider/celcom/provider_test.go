package celcom

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProviderCheckStatusDecodesFlatDeliveryReport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/services/getdlr/" {
			t.Errorf("request path = %q, want %q", r.URL.Path, "/api/services/getdlr/")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"response-code":200,"response-description":"DELIVERED","messageid":"provider-message-id"}`)
	}))
	defer server.Close()

	provider := NewProvider(&Client{
		BaseURL:    server.URL,
		APIKey:     "api-key",
		PartnerID:  "partner-id",
		HTTPClient: server.Client(),
	})

	response, err := provider.CheckStatus(context.Background(), "provider-message-id")
	if err != nil {
		t.Fatalf("CheckStatus() error = %v", err)
	}
	if response.Status != "delivered" {
		t.Errorf("CheckStatus() status = %q, want delivered", response.Status)
	}
	if response.ProviderMsgID != "provider-message-id" {
		t.Errorf("CheckStatus() provider message id = %q, want provider-message-id", response.ProviderMsgID)
	}
}
