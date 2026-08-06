package sms

import (
	"strings"

	"github.com/labstack/echo/v5"

	backofficesms "github.com/coffeyvidzro/dugble/server/internal/backoffice/sms"
	backofficehttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice"
)

type Handler struct {
	service *backofficesms.Service
}

func NewHandler(service *backofficesms.Service) *Handler {
	return &Handler{service: service}
}

func (handler *Handler) List(c *echo.Context) error {
	filter := backofficesms.Filter{
		Query:  strings.TrimSpace(c.QueryParam("q")),
		Status: strings.TrimSpace(c.QueryParam("status")),
	}

	messages, err := handler.service.List(c.Request().Context(), filter)
	if err != nil {
		return err
	}

	return backofficehttp.RenderPage(c, "sms.html", backofficehttp.PageData{
		Title:  "SMS",
		Data:   messages,
		Filter: filter,
	})
}

func (handler *Handler) Detail(c *echo.Context) error {
	detail, err := handler.service.Detail(c.Request().Context(), c.Param("sms_id"))
	if err != nil {
		return err
	}

	return backofficehttp.RenderPage(c, "sms_detail.html", backofficehttp.PageData{
		Title: "SMS Message",
		Data:  detail,
	})
}
