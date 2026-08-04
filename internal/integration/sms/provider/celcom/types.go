package celcom

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type SendRequest struct {
	PartnerID string `json:"partnerID"`
	APIKey    string `json:"apikey"`
	Mobile    string `json:"mobile"`
	Message   string `json:"message"`
	Shortcode string `json:"shortcode"`
	PassType  string `json:"pass_type"`
}

type DeliveryReportRequest struct {
	PartnerID string `json:"partnerID"`
	APIKey    string `json:"apikey"`
	MessageID string `json:"messageID"`
}

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

// DeliveryReportResponse is the flat response returned by the getdlr endpoint.
// Unlike SendResponse, it is not wrapped in a "responses" array.
type DeliveryReportResponse struct {
	ResponseCode        int
	ResponseDescription string
	Mobile              string
	MessageID           string
	NetworkID           string
}

func (r *DeliveryReportResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return fmt.Errorf("celcom delivery report response is nil")
	}

	var result SendResult
	if err := json.Unmarshal(data, &result); err != nil {
		return err
	}
	r.ResponseCode = result.ResponseCode
	r.ResponseDescription = result.ResponseDescription
	r.Mobile = result.Mobile
	r.MessageID = result.MessageID
	r.NetworkID = result.NetworkID
	return nil
}

func (r *SendResult) UnmarshalJSON(data []byte) error {
	if r == nil {
		return fmt.Errorf("celcom send result is nil")
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	var err error
	if raw := firstRaw(fields, "respose-code", "response-code", "response_code", "code"); raw != nil {
		r.ResponseCode, err = rawInt(raw)
		if err != nil {
			return fmt.Errorf("decode response code: %w", err)
		}
	}
	if raw := firstRaw(fields, "response-description", "response_description", "description", "message"); raw != nil {
		r.ResponseDescription, err = rawString(raw)
		if err != nil {
			return fmt.Errorf("decode response description: %w", err)
		}
	}
	if raw := firstRaw(fields, "mobile", "recipient"); raw != nil {
		r.Mobile, err = rawString(raw)
		if err != nil {
			return fmt.Errorf("decode mobile: %w", err)
		}
	}
	if raw := firstRaw(fields, "messageid", "messageID", "message_id", "id"); raw != nil {
		r.MessageID, err = rawString(raw)
		if err != nil {
			return fmt.Errorf("decode message id: %w", err)
		}
	}
	if raw := firstRaw(fields, "networkid", "networkID", "network_id"); raw != nil {
		r.NetworkID, err = rawString(raw)
		if err != nil {
			return fmt.Errorf("decode network id: %w", err)
		}
	}
	return nil
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
