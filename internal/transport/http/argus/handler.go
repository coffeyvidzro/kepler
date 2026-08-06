package argus

import (
	"crypto/sha256"
	"encoding/json"
	"net"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/idempotency"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (handler *Handler) Create(c *echo.Context) error {
	if err := requireIdempotencyKey(c); err != nil {
		return err
	}
	var req CreateVerificationRequest
	if err := decodeJSON(c, &req); err != nil {
		return err
	}
	result, err := handler.service.Create(c.Request().Context(), req)
	if err != nil {
		return httputil.Error(c, err)
	}
	c.Response().Header().Set("Location", "/verifications/"+result.ID)
	return httputil.Accepted(c, result)
}

func (handler *Handler) List(c *echo.Context) error {
	result, err := handler.service.List(c.Request().Context(), listRequest(c))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, result)
}

func (handler *Handler) Get(c *echo.Context) error {
	result, err := handler.service.Get(c.Request().Context(), c.Param("verification_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, result)
}

func (handler *Handler) Check(c *echo.Context) error {
	if err := requireIdempotencyKey(c); err != nil {
		return err
	}
	var req CheckRequest
	if err := decodeJSON(c, &req); err != nil {
		return err
	}
	userAgent := strings.TrimSpace(c.Request().UserAgent())
	if userAgent != "" {
		req.UserAgent = &userAgent
	}
	req.IPHash = requestIPHash(c)
	result, err := handler.service.Check(c.Request().Context(), c.Param("verification_id"), req)
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, result)
}

func (handler *Handler) Resend(c *echo.Context) error {
	if err := requireIdempotencyKey(c); err != nil {
		return err
	}
	result, err := handler.service.Resend(c.Request().Context(), c.Param("verification_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.Accepted(c, result)
}

func (handler *Handler) Cancel(c *echo.Context) error {
	result, err := handler.service.Cancel(c.Request().Context(), c.Param("verification_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, result)
}

func decodeJSON(c *echo.Context, destination any) error {
	decoder := json.NewDecoder(c.Request().Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return httputil.Error(c, apperrors.NewBadRequest("Invalid JSON request body"))
	}
	return nil
}

func requireIdempotencyKey(c *echo.Context) error {
	if _, err := idempotency.ValidateKey(c.Request().Header.Get(idempotency.Header)); err != nil {
		return httputil.Error(c, apperrors.NewBadRequest("Idempotency-Key is required and must be at most 256 characters"))
	}
	return nil
}

func requestIPHash(c *echo.Context) []byte {
	remoteAddress := strings.TrimSpace(c.Request().RemoteAddr)
	if host, _, err := net.SplitHostPort(remoteAddress); err == nil {
		remoteAddress = host
	}
	ip := net.ParseIP(strings.TrimSpace(remoteAddress))
	if ip == nil {
		return nil
	}
	hash := sha256.Sum256([]byte(ip.String()))
	return hash[:]
}

func listRequest(c *echo.Context) ListRequest {
	return ListRequest{Limit: parseInt32(c.QueryParam("limit")), Offset: parseInt32(c.QueryParam("offset"))}
}

func parseInt32(value string) int32 {
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0
	}
	return int32(parsed)
}
