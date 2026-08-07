package billingmarkets

import (
	"encoding/json"
	"strconv"

	"github.com/labstack/echo/v5"

	backofficebillingmarkets "github.com/coffeyvidzro/dugble/server/internal/backoffice/billingmarkets"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

type Handler struct {
	service *backofficebillingmarkets.Service
}

func NewHandler(service *backofficebillingmarkets.Service) *Handler {
	return &Handler{service: service}
}

func (handler *Handler) List(c *echo.Context) error {
	limit, err := parseInt32(c.QueryParam("limit"))
	if err != nil {
		return httputil.Error(c, apperrors.NewBadRequest("Invalid limit"))
	}
	offset, err := parseInt32(c.QueryParam("offset"))
	if err != nil {
		return httputil.Error(c, apperrors.NewBadRequest("Invalid offset"))
	}
	page, err := handler.service.List(c.Request().Context(), backofficebillingmarkets.ListInput{Limit: limit, Offset: offset})
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, page)
}

func (handler *Handler) Get(c *echo.Context) error {
	item, err := handler.service.Get(c.Request().Context(), c.Param("market_code"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, item)
}

func (handler *Handler) Create(c *echo.Context) error {
	var input backofficebillingmarkets.CreateInput
	if err := json.NewDecoder(c.Request().Body).Decode(&input); err != nil {
		return httputil.Error(c, apperrors.NewBadRequest("Invalid JSON request body"))
	}
	item, err := handler.service.Create(c.Request().Context(), input)
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.Created(c, item)
}

func (handler *Handler) Update(c *echo.Context) error {
	var input backofficebillingmarkets.UpdateInput
	if err := json.NewDecoder(c.Request().Body).Decode(&input); err != nil {
		return httputil.Error(c, apperrors.NewBadRequest("Invalid JSON request body"))
	}
	item, err := handler.service.Update(c.Request().Context(), c.Param("market_code"), input)
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, item)
}

func parseInt32(value string) (int32, error) {
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	return int32(parsed), err
}
