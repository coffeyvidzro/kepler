package sms

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/coffeyvidzro/dugble/server/internal/adapters/mnotify"
	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

func TestProviderSendsAndChecksStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("key") != "api-key" {
			t.Fatalf("key = %q", request.URL.Query().Get("key"))
		}
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case sendPath:
			var payload sendRequest
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatalf("decode send request: %v", err)
			}
			if len(payload.Recipient) != 1 || payload.Recipient[0] != "233201234567" {
				t.Fatalf("recipient = %#v", payload.Recipient)
			}
			if payload.Sender != "Dugble" || payload.Message != "Hello" || payload.IsSchedule {
				t.Fatalf("payload = %#v", payload)
			}
			_, _ = response.Write([]byte(`{"status":"success","code":2000,"message":"messages sent successfully","summary":{"_id":"campaign-1","total_sent":1,"contacts":1,"total_rejected":0}}`))
		case statusPath + "campaign-1":
			_, _ = response.Write([]byte(`{"status":"success","report":[{"campaign_id":"campaign-1","recipient":"233201234567","status":"DELIVRD"}]}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	provider := NewProvider(mnotify.NewClientWithHTTP("api-key", testHTTPClient(t, server)))
	sent, err := provider.Send(context.Background(), platformsms.SendRequest{
		To:                 "+233201234567",
		From:               "Dugble",
		Message:            "Hello",
		DestinationCountry: platformsms.CountryGhana,
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if sent.ProviderID != ProviderID || sent.ProviderMsgID != "campaign-1" || sent.Status != platformsms.StatusSubmitted {
		t.Fatalf("sent = %#v", sent)
	}

	status, err := provider.CheckStatus(context.Background(), sent.ProviderMsgID)
	if err != nil {
		t.Fatalf("CheckStatus() error = %v", err)
	}
	if status.Status != platformsms.StatusDelivered || status.ProviderStatus != "DELIVRD" {
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
