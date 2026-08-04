package arkesel

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/labstack/echo/v5"

	httptransport "github.com/coffeyvidzro/dugble/server/internal/transport/http"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

type DeliveryReport struct {
	MessageID string `json:"id"`
	Status string `json:"status"`
	Sender string `json:"sender,omitempty"`
	Recipient string `json:"recipient"`
	Message string `json:"message,omitempty"`
	SentAt string `json:"sent_at_time,omitempty"`
	Raw json.RawMessage `json:"-"`
}

type Ingestor interface { IngestArkesel(context.Context, DeliveryReport) error }
type Handler struct{ ingestor Ingestor }
func NewHandler(ingestor Ingestor) *Handler { return &Handler{ingestor:ingestor} }
func (handler *Handler) Receive(c *echo.Context) error {
	body,err:=httptransport.ReadBody(c,256*1024);if err!=nil{return httptransport.Error(c,err)}
	var report DeliveryReport;if err:=json.Unmarshal(body,&report);err!=nil{return httptransport.Error(c,apperrors.NewBadRequest("Invalid Arkesel delivery report"))}
	report.MessageID=strings.TrimSpace(report.MessageID);report.Status=strings.TrimSpace(report.Status);report.Recipient=strings.TrimSpace(report.Recipient);report.Raw=append(json.RawMessage(nil),body...)
	if report.MessageID==""||report.Status==""||report.Recipient==""{return httptransport.Error(c,apperrors.NewBadRequest("Arkesel delivery report is incomplete"))}
	if handler==nil||handler.ingestor==nil{return httptransport.Error(c,apperrors.NewServiceUnavailable("Arkesel delivery report ingestion is not configured",nil))}
	if err:=handler.ingestor.IngestArkesel(c.Request().Context(),report);err!=nil{return httptransport.Error(c,apperrors.NewServiceUnavailable("Unable to accept Arkesel delivery report",err))}
	return httptransport.NoContent(c)
}
