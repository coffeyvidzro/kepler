package csrf

import (
	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/transport/middlewares"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

type Handler struct{}

type TokenResponse struct {
	Token string `json:"csrf_token"`
}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) Token(c *echo.Context) error {
	token, ok := c.Get(middlewares.CSRFContextKey).(string)
	if !ok || token == "" {
		return httputil.Error(c, apperrors.NewInternal("CSRF token is not available", nil))
	}

	return httputil.OK(c, TokenResponse{Token: token})
}
