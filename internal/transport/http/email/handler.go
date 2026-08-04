package email

import (
	"github.com/labstack/echo/v5"

	emailmodule "github.com/coffeyvidzro/dugble/server/internal/modules/email"
	"github.com/coffeyvidzro/dugble/server/internal/platform/idempotency"
	httptransport "github.com/coffeyvidzro/dugble/server/internal/transport/http"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

type Handler struct {
	service *emailmodule.Service
}

func NewHandler(service *emailmodule.Service) *Handler {
	return &Handler{service: service}
}

func (handler *Handler) Send(c *echo.Context) error {
	if handler == nil || handler.service == nil {
		return httptransport.Error(c, apperrors.NewServiceUnavailable("Email service is not configured", nil))
	}
	if _, err := idempotency.ValidateKey(c.Request().Header.Get(idempotency.Header)); err != nil {
		return httptransport.Error(c, apperrors.NewBadRequest("Idempotency-Key is required and must be at most 256 characters"))
	}
	var request emailmodule.SendRequest
	if err := httptransport.DecodeJSON(c, &request, httptransport.DefaultMaxRequestBodyBytes); err != nil {
		return httptransport.Error(c, err)
	}
	message, err := handler.service.Send(c.Request().Context(), request)
	if err != nil {
		return httptransport.Error(c, err)
	}
	c.Response().Header().Set("Location", "/emails/"+message.ID)
	return httptransport.Accepted(c, message.Summary())
}

func (handler *Handler) Get(c *echo.Context) error {
	message, err := handler.service.Get(c.Request().Context(), c.Param("message_id"))
	if err != nil {
		return httptransport.Error(c, err)
	}
	return httptransport.OK(c, message.RetrieveResponse())
}

func (handler *Handler) Cancel(c *echo.Context) error {
	response, err := handler.service.Cancel(c.Request().Context(), c.Param("message_id"))
	if err != nil {
		return httptransport.Error(c, err)
	}
	return httptransport.OK(c, response)
}

func (handler *Handler) Update(c *echo.Context) error {
	var request emailmodule.UpdateRequest
	if err := httptransport.DecodeJSON(c, &request, httptransport.DefaultMaxRequestBodyBytes); err != nil {
		return httptransport.Error(c, err)
	}
	response, err := handler.service.Update(c.Request().Context(), c.Param("message_id"), request)
	if err != nil {
		return httptransport.Error(c, err)
	}
	return httptransport.OK(c, response)
}

func (handler *Handler) List(c *echo.Context) error {
	messages, err := handler.service.List(c.Request().Context(), emailmodule.ListRequest{
		Limit: httptransport.QueryInt32(c, "limit"), Offset: httptransport.QueryInt32(c, "offset"),
	})
	if err != nil {
		return httptransport.Error(c, err)
	}
	return httptransport.OK(c, messages)
}

func (handler *Handler) BatchSend(c *echo.Context) error {
	var request emailmodule.BatchSendRequest
	if err := httptransport.DecodeJSON(c, &request, httptransport.DefaultMaxRequestBodyBytes); err != nil {
		return httptransport.Error(c, err)
	}
	messages, err := handler.service.BatchSend(c.Request().Context(), request)
	if err != nil {
		return httptransport.Error(c, err)
	}
	return httptransport.Accepted(c, emailmodule.SendResponses(messages))
}
