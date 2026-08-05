package celcom

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

type SendResponse struct {
	Responses []SendResult `json:"responses"`
}

type SendResult struct {
	ResponseCode        int
	ResponseDescription string
	Mobile              string
	MessageID           string
	NetworkID           string
}

type DeliveryReportResponse struct {
	ResponseCode        int
	ResponseDescription string
	Mobile              string
	MessageID           string
	NetworkID           string
}

func (response *DeliveryReportResponse) UnmarshalJSON(data []byte) error {
	if response == nil {
		return fmt.Errorf("%w: delivery report target is nil", ErrInvalidResponse)
	}
	var result SendResult
	if err := json.Unmarshal(data, &result); err != nil {
		return err
	}
	response.ResponseCode = result.ResponseCode
	response.ResponseDescription = result.ResponseDescription
	response.Mobile = result.Mobile
	response.MessageID = result.MessageID
	response.NetworkID = result.NetworkID
	return nil
}

func (result *SendResult) UnmarshalJSON(data []byte) error {
	if result == nil {
		return fmt.Errorf("%w: send result target is nil", ErrInvalidResponse)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	var err error
	if raw := firstRaw(fields, "respose-code", "response-code", "response_code", "code"); raw != nil {
		result.ResponseCode, err = rawInt(raw)
		if err != nil {
			return fmt.Errorf("decode response code: %w", err)
		}
	}
	if raw := firstRaw(fields, "response-description", "response_description", "description", "message"); raw != nil {
		result.ResponseDescription, err = rawString(raw)
		if err != nil {
			return fmt.Errorf("decode response description: %w", err)
		}
	}
	if raw := firstRaw(fields, "mobile", "recipient"); raw != nil {
		result.Mobile, err = rawString(raw)
		if err != nil {
			return fmt.Errorf("decode mobile: %w", err)
		}
	}
	if raw := firstRaw(fields, "messageid", "messageID", "message_id", "id"); raw != nil {
		result.MessageID, err = rawString(raw)
		if err != nil {
			return fmt.Errorf("decode message ID: %w", err)
		}
	}
	if raw := firstRaw(fields, "networkid", "networkID", "network_id"); raw != nil {
		result.NetworkID, err = rawString(raw)
		if err != nil {
			return fmt.Errorf("decode network ID: %w", err)
		}
	}
	return nil
}

func mapSendResponse(response *SendResponse) (*platformsms.SendResponse, error) {
	if response == nil {
		return nil, fmt.Errorf("%w: send response is nil", ErrInvalidResponse)
	}
	if len(response.Responses) == 0 {
		return nil, fmt.Errorf("%w: send response contains no results", ErrInvalidResponse)
	}
	result := response.Responses[0]
	if result.ResponseCode != 200 {
		return nil, &APIError{Code: result.ResponseCode, Description: result.ResponseDescription}
	}
	messageID := strings.TrimSpace(result.MessageID)
	if messageID == "" {
		return nil, fmt.Errorf("%w: send response contains no message ID", ErrInvalidResponse)
	}
	return &platformsms.SendResponse{ProviderID: ProviderID, ProviderMsgID: messageID, Status: platformsms.StatusSubmitted}, nil
}

func mapStatusResponse(messageID string, response *DeliveryReportResponse) (*platformsms.StatusResponse, error) {
	if response == nil {
		return nil, fmt.Errorf("%w: delivery report response is nil", ErrInvalidResponse)
	}
	if response.ResponseCode != 200 {
		if response.ResponseCode == 1008 {
			return &platformsms.StatusResponse{ProviderID: ProviderID, ProviderMsgID: strings.TrimSpace(messageID), Status: platformsms.StatusUnknown}, nil
		}
		return nil, &APIError{Code: response.ResponseCode, Description: response.ResponseDescription}
	}
	return &platformsms.StatusResponse{
		ProviderID:    ProviderID,
		ProviderMsgID: strings.TrimSpace(messageID),
		Status:        normalizeStatus(response.ResponseDescription),
	}, nil
}

func normalizeStatus(value string) string {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	switch {
	case strings.Contains(normalized, "DELIVERED"), normalized == "DELIVRD":
		return platformsms.StatusDelivered
	case strings.Contains(normalized, "UNDELIVERED"), normalized == "UNDELIV":
		return platformsms.StatusUndelivered
	case strings.Contains(normalized, "REJECT"):
		return platformsms.StatusRejected
	case strings.Contains(normalized, "EXPIRED"):
		return platformsms.StatusExpired
	case strings.Contains(normalized, "FAILED"), strings.Contains(normalized, "ERROR"):
		return platformsms.StatusFailed
	case normalized == "SENT":
		return platformsms.StatusSent
	case strings.Contains(normalized, "SUBMIT"), strings.Contains(normalized, "QUEUE"), strings.Contains(normalized, "PENDING"), strings.Contains(normalized, "SUCCESS"):
		return platformsms.StatusSubmitted
	default:
		return platformsms.StatusUnknown
	}
}

func firstRaw(fields map[string]json.RawMessage, keys ...string) json.RawMessage {
	for _, key := range keys {
		if raw, ok := fields[key]; ok {
			return raw
		}
	}
	return nil
}

func rawInt(raw json.RawMessage) (int, error) {
	value, err := rawString(raw)
	if err != nil {
		return 0, err
	}
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q", value)
	}
	return parsed, nil
}

func rawString(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", nil
	}
	var text string
	if raw[0] == '"' {
		if err := json.Unmarshal(raw, &text); err != nil {
			return "", err
		}
		return strings.TrimSpace(text), nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	if number, ok := value.(json.Number); ok {
		return number.String(), nil
	}
	return "", fmt.Errorf("expected string or number")
}
