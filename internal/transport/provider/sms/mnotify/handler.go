package mnotify

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/labstack/echo/v5"

	httptransport "github.com/coffeyvidzro/dugble/server/internal/transport/http"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

type DeliveryReport struct {
	ID         int64           `json:"_id"`
	Recipient  string          `json:"recipient"`
	Message    string          `json:"message,omitempty"`
	Sender     string          `json:"sender,omitempty"`
	Status     string          `json:"status"`
	DateSent   string          `json:"date_sent,omitempty"`
	CampaignID string          `json:"campaign_id"`
	Retries    int             `json:"retries,omitempty"`
	Raw        json.RawMessage `json:"-"`
}

type Ingestor interface {
	IngestMNotify(context.Context, DeliveryReport) error
}

type Handler struct{ ingestor Ingestor }

func NewHandler(ingestor Ingestor) *Handler { return &Handler{ingestor: ingestor} }

func (handler *Handler) Receive(c *echo.Context) error {
	body, err := httptransport.ReadBody(c, 256*1024)
	if err != nil {
		return httptransport.Error(c, err)
	}
	var report DeliveryReport
	if err := json.Unmarshal(body, &report); err != nil {
		return httptransport.Error(c, apperrors.NewBadRequest("Invalid mNotify delivery report"))
	}
	report.Recipient = strings.TrimSpace(report.Recipient)
	report.Status = strings.TrimSpace(report.Status)
	report.CampaignID = strings.TrimSpace(report.CampaignID)
	report.Raw = append(json.RawMessage(nil), body...)
	if report.Recipient == "" || report.Status == "" || report.CampaignID == "" {
		return httptransport.Error(c, apperrors.NewBadRequest("mNotify delivery report is incomplete"))
	}
	if handler == nil || handler.ingestor == nil {
		return httptransport.Error(c, apperrors.NewServiceUnavailable("mNotify delivery report ingestion is not configured", nil))
	}
	if err := handler.ingestor.IngestMNotify(c.Request().Context(), report); err != nil {
		return httptransport.Error(c, apperrors.NewServiceUnavailable("Unable to accept mNotify delivery report", err))
	}
	return httptransport.NoContent(c)
}
