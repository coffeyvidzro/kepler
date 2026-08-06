package sender

import (
	"fmt"
	"strings"

	platformsenderid "github.com/coffeyvidzro/dugble/server/internal/platform/senderid"
)

type CreateRequest = platformsenderid.CreateRequest

type createRequest struct {
	SenderName string `json:"sender_name"`
	Purpose    string `json:"purpose"`
}

func validateCreateRequest(request platformsenderid.CreateRequest) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(request.Purpose) == "" {
		return fmt.Errorf("sender ID purpose is required")
	}
	return nil
}

func newCreateRequest(request platformsenderid.CreateRequest) createRequest {
	request = request.Normalize()
	return createRequest{SenderName: request.SenderID, Purpose: request.Purpose}
}

type statusRequest struct {
	SenderName string `json:"sender_name"`
}

func newStatusRequest(senderID string) statusRequest {
	return statusRequest{SenderName: platformsenderid.NormalizeName(senderID)}
}
