package middlewares

import (
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
	"github.com/newrelic/go-agent/v3/newrelic"
)

// NewRelic names the transaction with Echo's bounded route template and records handler errors.
func NewRelic() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			txn := newrelic.FromContext(c.Request().Context())
			if txn != nil {
				path := strings.TrimSpace(c.RouteInfo().Path)
				if path == "" {
					path = "unmatched"
				}
				txn.SetName(c.Request().Method + " " + path)
				txn.AddAttribute("http.route", path)
			}

			err := next(c)
			if txn != nil {
				if response, ok := c.Response().(*echo.Response); ok {
					txn.AddAttribute("http.statusCode", response.Status)
				}
				if shouldNoticeError(err) {
					txn.NoticeError(err)
				}
			}
			return err
		}
	}
}

func shouldNoticeError(err error) bool {
	if err == nil {
		return false
	}

	var httpError *echo.HTTPError
	if errors.As(err, &httpError) {
		return httpError.Code >= http.StatusInternalServerError
	}
	return true
}
