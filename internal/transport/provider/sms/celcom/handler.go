package celcom

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/labstack/echo/v5"

	httptransport "github.com/coffeyvidzro/dugble/server/internal/transport/http"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

type DeliveryReport struct {
	ResponseCode        int             `json:"response_code"`
	ResponseDescription string          `json:"response_description"`
	Mobile              string          `json:"mobile"`
	MessageID           string          `json:"message_id"`
	NetworkID           string          `json:"network_id,omitempty"`
	Raw                 json.RawMessage `json:"-"`
}

type Ingestor interface {
	IngestCelcom(context.Context, DeliveryReport) error
}

type Handler struct{ ingestor Ingestor }

func NewHandler(ingestor Ingestor) *Handler { return &Handler{ingestor: ingestor} }

func (handler *Handler) Receive(c *echo.Context) error {
	body, err := httptransport.ReadBody(c, 256*1024)
	if err != nil {
		return httptransport.Error(c, err)
	}
	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		return httptransport.Error(c, apperrors.NewBadRequest("Invalid Celcom delivery report"))
	}
	report := DeliveryReport{
		ResponseCode:        intValue(fields, "response_code", "response-code", "respose-code", "code"),
		ResponseDescription: stringValue(fields, "response_description", "response-description", "description", "message"),
		Mobile:              stringValue(fields, "mobile", "recipient"),
		MessageID:           stringValue(fields, "message_id", "messageID", "messageid", "id"),
		NetworkID:           stringValue(fields, "network_id", "networkID", "networkid"),
		Raw:                 append(json.RawMessage(nil), body...),
	}
	if strings.TrimSpace(report.MessageID) == "" || strings.TrimSpace(report.Mobile) == "" {
		return httptransport.Error(c, apperrors.NewBadRequest("Celcom delivery report is incomplete"))
	}
	if handler == nil || handler.ingestor == nil {
		return httptransport.Error(c, apperrors.NewServiceUnavailable("Celcom delivery report ingestion is not configured", nil))
	}
	if err := handler.ingestor.IngestCelcom(c.Request().Context(), report); err != nil {
		return httptransport.Error(c, apperrors.NewServiceUnavailable("Unable to accept Celcom delivery report", err))
	}
	return httptransport.NoContent(c)
}

func stringValue(fields map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, exists := fields[key]; exists {
			return strings.TrimSpace(toString(value))
		}
	}
	return ""
}

func intValue(fields map[string]any, keys ...string) int {
	for _, key := range keys {
		if value, exists := fields[key]; exists {
			switch typed := value.(type) {
			case float64:
				return int(typed)
			case json.Number:
				parsed, _ := typed.Int64()
				return int(parsed)
			}
		}
	}
	return 0
}

func toString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return json.Number(strings.TrimRight(strings.TrimRight(fmtFloat(typed), "0"), ".")).String()
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

func fmtFloat(value float64) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
