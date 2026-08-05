package auth

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

type Handler struct {
	service      *Service
	development  bool
	cookieDomain string
}

func NewHandler(service *Service, development bool, cookieDomains ...string) *Handler {
	cookieDomain := ""
	if len(cookieDomains) > 0 {
		cookieDomain = strings.TrimSpace(cookieDomains[0])
	}

	return &Handler{
		service:      service,
		development:  development,
		cookieDomain: cookieDomain,
	}
}

func (h *Handler) GetUser(c *echo.Context) error {
	response, err := h.service.GetUser(c.Request().Context())
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, response)
}

func (h *Handler) Register(c *echo.Context) error {
	var req RegisterRequest
	if err := decodeJSON(c, &req); err != nil {
		return err
	}
	response, err := h.service.Register(
		c.Request().Context(),
		req,
	)
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.Created(c, response)
}

func (h *Handler) Login(c *echo.Context) error {
	var req LoginRequest
	if err := decodeJSON(c, &req); err != nil {
		return err
	}
	response, token, expiresAt, err := h.service.Login(
		c.Request().Context(),
		req,
		stringPtr(c.Request().UserAgent()),
		stringPtr(clientIP(c)),
	)
	if err != nil {
		return httputil.Error(c, err)
	}
	if token != "" {
		h.setSessionCookie(c, token, expiresAt)
	}
	return httputil.OK(c, response)
}

func (h *Handler) CompleteMFATOTP(c *echo.Context) error     { return h.completeMFALogin(c, false) }
func (h *Handler) CompleteMFARecovery(c *echo.Context) error { return h.completeMFALogin(c, true) }

func (h *Handler) completeMFALogin(c *echo.Context, recovery bool) error {
	var req MFALoginRequest
	if err := decodeJSON(c, &req); err != nil {
		return err
	}
	response, token, expiresAt, err := h.service.CompleteMFALogin(c.Request().Context(), req, recovery, stringPtr(c.Request().UserAgent()), stringPtr(clientIP(c)))
	if err != nil {
		return httputil.Error(c, err)
	}
	h.setSessionCookie(c, token, expiresAt)
	return httputil.OK(c, response)
}

func (h *Handler) VerifyEmail(c *echo.Context) error {
	var req VerifyEmailRequest
	if err := decodeJSON(c, &req); err != nil {
		return err
	}
	if err := h.service.VerifyEmail(c.Request().Context(), req); err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, map[string]bool{"email_verified": true})
}

func (h *Handler) ResendEmail(c *echo.Context) error {
	var req ResendEmailRequest
	if err := decodeJSON(c, &req); err != nil {
		return err
	}
	if err := h.service.ResendEmail(c.Request().Context(), req); err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, map[string]bool{"sent": true})
}

func (h *Handler) ForgotPassword(c *echo.Context) error {
	var req ForgotPasswordRequest
	if err := decodeJSON(c, &req); err != nil {
		return err
	}
	if err := h.service.ForgotPassword(c.Request().Context(), req); err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, map[string]bool{"sent": true})
}

func (h *Handler) ResetPassword(c *echo.Context) error {
	var req ResetPasswordRequest
	if err := decodeJSON(c, &req); err != nil {
		return err
	}
	if err := h.service.ResetPassword(c.Request().Context(), req); err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, map[string]bool{"password_reset": true})
}

func (h *Handler) Logout(c *echo.Context) error {
	if err := h.service.Logout(c.Request().Context()); err != nil {
		return httputil.Error(c, err)
	}
	cookie := expiredSessionCookie(h.development, h.cookieDomain)
	c.SetCookie(cookie)
	return httputil.OK(c, map[string]bool{"logged_out": true})
}

func decodeJSON(c *echo.Context, dst any) error {
	if err := json.NewDecoder(c.Request().Body).Decode(dst); err != nil {
		return httputil.Error(c, apperrors.NewBadRequest("Invalid JSON request body"))
	}
	return nil
}

func (h *Handler) setSessionCookie(c *echo.Context, token string, expiresAt time.Time) {
	c.SetCookie(
		&http.Cookie{
			Name:     authnz.SessionCookieName,
			Value:    token,
			Path:     "/",
			Domain:   h.cookieDomain,
			Expires:  expiresAt,
			HttpOnly: true,
			Secure:   !h.development,
			SameSite: http.SameSiteLaxMode,
		},
	)
}

func expiredSessionCookie(development bool, cookieDomain string) *http.Cookie {
	return &http.Cookie{
		Name:     authnz.SessionCookieName,
		Value:    "",
		Path:     "/",
		Domain:   strings.TrimSpace(cookieDomain),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   !development,
		SameSite: http.SameSiteLaxMode,
	}
}

func stringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func clientIP(c *echo.Context) string {
	if forwarded := strings.TrimSpace(
		c.Request().Header.Get(echo.HeaderXForwardedFor),
	); forwarded != "" {
		return strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	return c.Request().RemoteAddr
}
