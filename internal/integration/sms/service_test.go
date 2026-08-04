package sms

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateStatusResponseRejectsEmptyProviderID(t *testing.T) {
	response := &StatusResponse{
		ProviderID:    "  ",
		ProviderMsgID: "message-123",
		Status:        StatusDelivered,
	}

	err := validateStatusResponse("provider", "message-123", response)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !errors.Is(err, ErrInvalidProviderReply) {
		t.Fatalf("expected ErrInvalidProviderReply, got %v", err)
	}
	if !strings.Contains(err.Error(), "provider ID is empty") {
		t.Fatalf("expected empty provider ID error, got %q", err.Error())
	}
}
