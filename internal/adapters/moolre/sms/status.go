package sms

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/coffeyvidzro/dugble/server/internal/adapters/moolre"
	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

type statusEntry struct {
	Reference string `json:"ref"`
	Status    int    `json:"status"`
}

type statusResponse = moolre.Envelope[[]statusEntry]

func mapStatusResponse(references []string, response *statusResponse) ([]platformsms.StatusResponse, error) {
	if response == nil {
		return nil, fmt.Errorf("%w: status response is nil", moolre.ErrInvalidResponse)
	}
	if !response.Successful() {
		return nil, &moolre.APIError{
			Status:     response.Status,
			Code:       strings.TrimSpace(response.Code),
			Message:    response.Message.String(),
			Definitive: true,
		}
	}
	if !strings.EqualFold(strings.TrimSpace(response.Code), "ASMQ10") {
		return nil, fmt.Errorf("%w: successful status response has code %q", moolre.ErrInvalidResponse, response.Code)
	}

	requested := make(map[string]struct{}, len(references))
	for _, reference := range references {
		requested[reference] = struct{}{}
	}
	entries := make(map[string]statusEntry, len(response.Data))
	for _, entry := range response.Data {
		reference := strings.TrimSpace(entry.Reference)
		if reference == "" {
			return nil, fmt.Errorf("%w: status response contains an empty reference", moolre.ErrInvalidResponse)
		}
		if _, exists := requested[reference]; !exists {
			return nil, fmt.Errorf("%w: status response contains unrequested reference %q", moolre.ErrInvalidResponse, reference)
		}
		if _, exists := entries[reference]; exists {
			return nil, fmt.Errorf("%w: status response contains duplicate reference %q", moolre.ErrInvalidResponse, reference)
		}
		entry.Reference = reference
		entries[reference] = entry
	}

	result := make([]platformsms.StatusResponse, 0, len(references))
	for _, reference := range references {
		mapped := platformsms.StatusResponse{
			ProviderID:    ProviderID,
			ProviderMsgID: reference,
			Status:        platformsms.StatusUnknown,
		}
		if entry, exists := entries[reference]; exists {
			mapped.Status = normalizeStatus(entry.Status)
			mapped.ProviderStatus = strconv.Itoa(entry.Status)
		}
		result = append(result, mapped)
	}
	return result, nil
}

// Moolre's public SMS status documentation currently exposes numeric values but
// does not define their delivery semantics. Preserve the provider value and keep
// the normalized status unknown until an authoritative mapping is available.
func normalizeStatus(_ int) string { return platformsms.StatusUnknown }
