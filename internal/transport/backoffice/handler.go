package backoffice

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"

	"github.com/google/uuid"

	backofficedashboard "github.com/coffeyvidzro/dugble/server/internal/backoffice/dashboard"
	backofficedomains "github.com/coffeyvidzro/dugble/server/internal/backoffice/domains"
	backofficesenderids "github.com/coffeyvidzro/dugble/server/internal/backoffice/senderids"
	backofficesms "github.com/coffeyvidzro/dugble/server/internal/backoffice/sms"
	backofficeteams "github.com/coffeyvidzro/dugble/server/internal/backoffice/teams"
	backofficeusers "github.com/coffeyvidzro/dugble/server/internal/backoffice/users"
	"github.com/coffeyvidzro/dugble/server/internal/transport/middlewares"
)

type Handler struct {
	dashboard *backofficedashboard.Service
	users     *backofficeusers.Service
	sms       *backofficesms.Service
	teams     *backofficeteams.Service
	senderIDs *backofficesenderids.Service
	domains   *backofficedomains.Service
}

func NewHandler(
	dashboard *backofficedashboard.Service,
	users *backofficeusers.Service,
	sms *backofficesms.Service,
	teams *backofficeteams.Service,
	senderIDs *backofficesenderids.Service,
	domains *backofficedomains.Service,
) *Handler {
	return &Handler{dashboard: dashboard, users: users, sms: sms, teams: teams, senderIDs: senderIDs, domains: domains}
}

func (h *Handler) Dashboard(c *echo.Context) error {
	stats, err := h.dashboard.Stats(c.Request().Context())
	if err != nil {
		return err
	}

	return h.render(c, "dashboard.html", "Dashboard", stats, nil)
}

func (h *Handler) Users(c *echo.Context) error {
	filter := backofficeusers.Filter{Query: cleanQuery(c.QueryParam("q"))}
	users, err := h.users.List(c.Request().Context(), filter)
	if err != nil {
		return err
	}

	return h.render(c, "users.html", "Users", users, filter)
}

func (h *Handler) UserDetail(c *echo.Context) error {
	id, ok := validID(c)
	if !ok {
		return c.String(http.StatusBadRequest, "invalid user id")
	}

	detail, err := h.users.Detail(c.Request().Context(), id)
	if err != nil {
		return handleDetailError(c, err)
	}

	return h.render(c, "user_detail.html", detail.User.Email, detail, nil)
}

func (h *Handler) Teams(c *echo.Context) error {
	filter := backofficeteams.Filter{Query: cleanQuery(c.QueryParam("q"))}
	teams, err := h.teams.List(c.Request().Context(), filter)
	if err != nil {
		return err
	}

	return h.render(c, "teams.html", "Teams", teams, filter)
}

func (h *Handler) TeamDetail(c *echo.Context) error {
	id, ok := validID(c)
	if !ok {
		return c.String(http.StatusBadRequest, "invalid team id")
	}

	detail, err := h.teams.Detail(c.Request().Context(), id)
	if err != nil {
		return handleDetailError(c, err)
	}

	return h.render(c, "team_detail.html", detail.Team.Name, detail, nil)
}

func (h *Handler) UpdateTeamStatus(c *echo.Context) error {
	id, ok := validID(c)
	if !ok {
		return c.String(http.StatusBadRequest, "invalid team id")
	}

	if err := h.teams.UpdateStatus(c.Request().Context(), id, backofficeteams.StatusRequest{
		Action: c.Request().FormValue("action"),
		Reason: c.Request().FormValue("reason"),
	}); err != nil {
		return handleTeamCommandError(c, err)
	}

	return c.Redirect(http.StatusSeeOther, "/teams/"+id)
}

func (h *Handler) SMSMessages(c *echo.Context) error {
	filter := backofficesms.Filter{
		Query:  cleanQuery(c.QueryParam("q")),
		Status: cleanQuery(c.QueryParam("status")),
	}
	messages, err := h.sms.List(c.Request().Context(), filter)
	if err != nil {
		return err
	}

	return h.render(c, "sms.html", "SMS", messages, filter)
}

func (h *Handler) SMSDetail(c *echo.Context) error {
	id, ok := validID(c)
	if !ok {
		return c.String(http.StatusBadRequest, "invalid sms id")
	}

	detail, err := h.sms.Detail(c.Request().Context(), id)
	if err != nil {
		return handleDetailError(c, err)
	}

	return h.render(c, "sms_detail.html", "SMS "+detail.ID, detail, nil)
}

func (h *Handler) SenderIDs(c *echo.Context) error {
	filter := backofficesenderids.Filter{
		Query:  cleanQuery(c.QueryParam("q")),
		Status: cleanQuery(c.QueryParam("status")),
	}
	senderIDs, err := h.senderIDs.List(c.Request().Context(), filter)
	if err != nil {
		return err
	}

	return h.render(c, "sender_ids.html", "Sender IDs", senderIDs, filter)
}

func (h *Handler) SenderIDDetail(c *echo.Context) error {
	id, ok := validID(c)
	if !ok {
		return c.String(http.StatusBadRequest, "invalid sender id")
	}

	detail, err := h.senderIDs.Detail(c.Request().Context(), id)
	if err != nil {
		return handleDetailError(c, err)
	}

	return h.render(c, "sender_id_detail.html", "Sender ID "+detail.Name, detail, nil)
}

func (h *Handler) UpdateSenderIDStatus(c *echo.Context) error {
	id, ok := validID(c)
	if !ok {
		return c.String(http.StatusBadRequest, "invalid sender id")
	}

	if err := h.senderIDs.UpdateStatus(c.Request().Context(), id, backofficesenderids.StatusRequest{
		Action: c.Request().FormValue("action"),
		Reason: c.Request().FormValue("reason"),
	}); err != nil {
		return handleSenderIDCommandError(c, err)
	}

	return c.Redirect(http.StatusSeeOther, "/sender-ids/"+id)
}

func (h *Handler) Domains(c *echo.Context) error {
	filter := backofficedomains.Filter{
		Query:  cleanQuery(c.QueryParam("q")),
		Status: cleanQuery(c.QueryParam("status")),
	}
	domains, err := h.domains.List(c.Request().Context(), filter)
	if err != nil {
		return err
	}

	return h.render(c, "domains.html", "Domains", domains, filter)
}

func (h *Handler) DomainDetail(c *echo.Context) error {
	id, ok := validID(c)
	if !ok {
		return c.String(http.StatusBadRequest, "invalid domain id")
	}

	detail, err := h.domains.Detail(c.Request().Context(), id)
	if err != nil {
		return handleDetailError(c, err)
	}

	return h.render(c, "domain_detail.html", detail.Domain, detail, nil)
}

func (h *Handler) UpdateDomainStatus(c *echo.Context) error {
	id, ok := validID(c)
	if !ok {
		return c.String(http.StatusBadRequest, "invalid domain id")
	}

	if err := h.domains.UpdateStatus(c.Request().Context(), id, backofficedomains.StatusRequest{
		Action: c.Request().FormValue("action"),
		Reason: c.Request().FormValue("reason"),
	}); err != nil {
		return handleDomainCommandError(c, err)
	}

	return c.Redirect(http.StatusSeeOther, "/domains/"+id)
}

func cleanQuery(value string) string {
	return strings.TrimSpace(value)
}

func validID(c *echo.Context) (string, bool) {
	id := cleanQuery(c.Param("id"))
	if _, err := uuid.Parse(id); err != nil {
		return "", false
	}

	return id, true
}

func handleSenderIDCommandError(c *echo.Context, err error) error {
	if errors.Is(err, backofficesenderids.ErrInvalidRequest) {
		return c.String(http.StatusBadRequest, strings.TrimPrefix(err.Error(), backofficesenderids.ErrInvalidRequest.Error()+": "))
	}

	return handleDetailError(c, err)
}

func handleDomainCommandError(c *echo.Context, err error) error {
	if errors.Is(err, backofficedomains.ErrInvalidRequest) {
		return c.String(http.StatusBadRequest, strings.TrimPrefix(err.Error(), backofficedomains.ErrInvalidRequest.Error()+": "))
	}

	return handleDetailError(c, err)
}

func handleTeamCommandError(c *echo.Context, err error) error {
	if errors.Is(err, backofficeteams.ErrInvalidRequest) {
		return c.String(http.StatusBadRequest, strings.TrimPrefix(err.Error(), backofficeteams.ErrInvalidRequest.Error()+": "))
	}

	return handleDetailError(c, err)
}

func handleDetailError(c *echo.Context, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return c.String(http.StatusNotFound, "not found")
	}

	return err
}

func (h *Handler) render(c *echo.Context, templateName string, title string, data any, filter any) error {
	token, _ := c.Get(middlewares.CSRFContextKey).(string)

	return c.Render(http.StatusOK, templateName, PageData{
		Title:  title,
		Data:   data,
		Filter: filter,
		CSRF:   token,
	})
}
