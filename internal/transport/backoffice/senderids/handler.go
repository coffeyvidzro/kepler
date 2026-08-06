package senderids

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	backofficesenderids "github.com/coffeyvidzro/dugble/server/internal/backoffice/senderids"
	backofficehttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice"
)

type Handler struct {
	service *backofficesenderids.Service
}

func NewHandler(service *backofficesenderids.Service) *Handler {
	return &Handler{service: service}
}

func (handler *Handler) List(c *echo.Context) error {
	filter := backofficesenderids.Filter{
		Query:  strings.TrimSpace(c.QueryParam("q")),
		Status: strings.TrimSpace(c.QueryParam("status")),
	}

	senderIDs, err := handler.service.List(c.Request().Context(), filter)
	if err != nil {
		return err
	}

	return backofficehttp.RenderPage(c, "sender_ids.html", backofficehttp.PageData{
		Title:  "Sender IDs",
		Data:   senderIDs,
		Filter: filter,
	})
}

func (handler *Handler) Detail(c *echo.Context) error {
	detail, err := handler.service.Detail(c.Request().Context(), c.Param("sender_id"))
	if err != nil {
		return err
	}

	return backofficehttp.RenderPage(c, "sender_id_detail.html", backofficehttp.PageData{
		Title: "Sender ID",
		Data:  detail,
	})
}

func (handler *Handler) UpdateStatus(c *echo.Context) error {
	senderID := c.Param("sender_id")
	if err := handler.service.UpdateStatus(c.Request().Context(), senderID, backofficesenderids.StatusRequest{
		Action: c.FormValue("action"),
		Reason: c.FormValue("reason"),
	}); err != nil {
		return err
	}

	return c.Redirect(http.StatusSeeOther, "/sender-ids/"+senderID)
}
