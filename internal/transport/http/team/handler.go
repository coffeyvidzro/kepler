package team

import (
	"encoding/json"

	"github.com/labstack/echo/v5"

	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(c *echo.Context) error {
	teams, err := h.service.List(c.Request().Context())
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, teams)
}

func (h *Handler) Create(c *echo.Context) error {
	var req CreateRequest
	if err := decodeJSON(c, &req); err != nil {
		return err
	}
	team, err := h.service.Create(c.Request().Context(), req)
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.Created(c, team)
}

func (h *Handler) Get(c *echo.Context) error {
	team, err := h.service.Get(c.Request().Context(), c.Param("team_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, team)
}

func (h *Handler) Update(c *echo.Context) error {
	var req UpdateRequest
	if err := decodeJSON(c, &req); err != nil {
		return err
	}
	team, err := h.service.Update(c.Request().Context(), c.Param("team_id"), req)
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, team)
}

func (h *Handler) Delete(c *echo.Context) error {
	team, err := h.service.Delete(c.Request().Context(), c.Param("team_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, team)
}

func (h *Handler) ListMembers(c *echo.Context) error {
	members, err := h.service.ListMembers(c.Request().Context(), c.Param("team_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, members)
}

func (h *Handler) Leave(c *echo.Context) error {
	if err := h.service.Leave(c.Request().Context(), c.Param("team_id")); err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, map[string]bool{"left": true})
}

func (h *Handler) RemoveMember(c *echo.Context) error {
	if err := h.service.RemoveMember(
		c.Request().Context(),
		c.Param("team_id"),
		c.Param("user_id"),
	); err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, map[string]bool{"removed": true})
}

func (h *Handler) UpdateMemberRole(c *echo.Context) error {
	var req UpdateMemberRoleRequest
	if err := decodeJSON(c, &req); err != nil {
		return err
	}
	member, err := h.service.UpdateMemberRole(
		c.Request().Context(),
		c.Param("team_id"),
		c.Param("user_id"),
		req,
	)
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, member)
}

func (h *Handler) InviteMember(c *echo.Context) error {
	var req InviteMemberRequest
	if err := decodeJSON(c, &req); err != nil {
		return err
	}
	invitation, err := h.service.InviteMember(c.Request().Context(), c.Param("team_id"), req)
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.Created(c, invitation)
}

func (h *Handler) GetInvitation(c *echo.Context) error {
	invitation, err := h.service.GetInvitation(c.Request().Context(), c.Param("token"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, invitation)
}

func (h *Handler) AcceptInvitation(c *echo.Context) error {
	invitation, err := h.service.AcceptInvitation(c.Request().Context(), c.Param("token"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, invitation)
}

func (h *Handler) DeclineInvitation(c *echo.Context) error {
	invitation, err := h.service.DeclineInvitation(c.Request().Context(), c.Param("token"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, invitation)
}

func decodeJSON(c *echo.Context, dst any) error {
	if err := json.NewDecoder(c.Request().Body).Decode(dst); err != nil {
		return httputil.Error(c, apperrors.NewBadRequest("Invalid JSON request body"))
	}
	return nil
}
