package teams

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	backofficeteams "github.com/coffeyvidzro/dugble/server/internal/backoffice/teams"
	backofficehttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice"
)

type Handler struct {
	service *backofficeteams.Service
}

func NewHandler(service *backofficeteams.Service) *Handler {
	return &Handler{service: service}
}

func (handler *Handler) List(c *echo.Context) error {
	filter := backofficeteams.Filter{
		Query: strings.TrimSpace(c.QueryParam("q")),
	}

	teams, err := handler.service.List(c.Request().Context(), filter)
	if err != nil {
		return err
	}

	return backofficehttp.RenderPage(c, "teams.html", backofficehttp.PageData{
		Title:  "Teams",
		Data:   teams,
		Filter: filter,
	})
}

func (handler *Handler) Detail(c *echo.Context) error {
	detail, err := handler.service.Detail(c.Request().Context(), c.Param("team_id"))
	if err != nil {
		return err
	}

	return backofficehttp.RenderPage(c, "team_detail.html", backofficehttp.PageData{
		Title: "Team",
		Data:  detail,
	})
}

func (handler *Handler) UpdateStatus(c *echo.Context) error {
	teamID := c.Param("team_id")
	if err := handler.service.UpdateStatus(c.Request().Context(), teamID, backofficeteams.StatusRequest{
		Action: c.FormValue("action"),
		Reason: c.FormValue("reason"),
	}); err != nil {
		return err
	}

	return c.Redirect(http.StatusSeeOther, "/teams/"+teamID)
}
