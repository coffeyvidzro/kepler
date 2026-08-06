package users

import (
	"strings"

	"github.com/labstack/echo/v5"

	backofficeusers "github.com/coffeyvidzro/dugble/server/internal/backoffice/users"
	backofficehttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice"
)

type Handler struct {
	service *backofficeusers.Service
}

func NewHandler(service *backofficeusers.Service) *Handler {
	return &Handler{service: service}
}

func (handler *Handler) List(c *echo.Context) error {
	filter := backofficeusers.Filter{
		Query: strings.TrimSpace(c.QueryParam("q")),
	}

	users, err := handler.service.List(c.Request().Context(), filter)
	if err != nil {
		return err
	}

	return backofficehttp.RenderPage(c, "users.html", backofficehttp.PageData{
		Title:  "Users",
		Data:   users,
		Filter: filter,
	})
}

func (handler *Handler) Detail(c *echo.Context) error {
	detail, err := handler.service.Detail(c.Request().Context(), c.Param("user_id"))
	if err != nil {
		return err
	}

	return backofficehttp.RenderPage(c, "user_detail.html", backofficehttp.PageData{
		Title: "User",
		Data:  detail,
	})
}
