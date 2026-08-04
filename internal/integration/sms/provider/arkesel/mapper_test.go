package arkesel

import (
	"encoding/json"
	"testing"
)

func TestStatusResponseUnmarshalsLowercaseID(t *testing.T) {
	t.Parallel()

	payload := []byte(`{
		"status": "success",
		"data": {
			"id": "message-123",
			"status": "DELIVERED"
		}
	}`)

	var response StatusResponse
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatalf("unmarshal status response: %v", err)
	}

	internal, err := StatusToInternal(&response)
	if err != nil {
		t.Fatalf("StatusToInternal() error = %v", err)
	}
	if internal.ProviderMsgID != "message-123" {
		t.Fatalf("ProviderMsgID = %q, want %q", internal.ProviderMsgID, "message-123")
	}
}
