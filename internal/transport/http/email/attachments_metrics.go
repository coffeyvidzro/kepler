package email

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v5"

	emailmodule "github.com/coffeyvidzro/dugble/server/internal/modules/email"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

func (handler *Handler) ListAttachments(c *echo.Context) error {
	if handler == nil || handler.service == nil {
		return httputil.Error(c, apperrors.NewServiceUnavailable("Email service is not configured", nil))
	}
	response, err := handler.service.ListAttachments(c.Request().Context(), c.Param("message_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	for index := range response.Data {
		response.Data[index].DownloadURL = attachmentDownloadURL(c, c.Param("message_id"), response.Data[index].ID)
	}
	// These endpoints intentionally use the Resend wire response directly.
	return c.JSON(http.StatusOK, response)
}

func (handler *Handler) GetAttachment(c *echo.Context) error {
	if handler == nil || handler.service == nil {
		return httputil.Error(c, apperrors.NewServiceUnavailable("Email service is not configured", nil))
	}
	response, err := handler.service.GetAttachment(c.Request().Context(), c.Param("message_id"), c.Param("attachment_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	response.DownloadURL = attachmentDownloadURL(c, c.Param("message_id"), response.ID)
	return c.JSON(http.StatusOK, response)
}

func (handler *Handler) DownloadAttachment(c *echo.Context) error {
	if handler == nil || handler.service == nil {
		return httputil.Error(c, apperrors.NewServiceUnavailable("Email service is not configured", nil))
	}
	download, err := handler.service.DownloadAttachment(c.Request().Context(), c.Param("message_id"), c.Param("attachment_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	c.Response().Header().Set(echo.HeaderContentType, download.ContentType)
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename=%q`, download.ContentDisposition, download.Filename))
	c.Response().Header().Set(echo.HeaderCacheControl, "private, max-age=900")
	return c.Blob(http.StatusOK, download.ContentType, download.Content)
}

func (handler *Handler) Metrics(c *echo.Context) error {
	if handler == nil || handler.service == nil {
		return httputil.Error(c, apperrors.NewServiceUnavailable("Email service is not configured", nil))
	}
	response, err := handler.service.Metrics(c.Request().Context(), emailmodule.MetricsRequest{
		StartDate: c.QueryParam("start_date"), EndDate: c.QueryParam("end_date"),
		Timezone: c.QueryParam("timezone"), Granularity: c.QueryParam("granularity"),
		Metrics: commaValues(c.QueryParam("metrics")), Dimensions: commaValues(c.QueryParam("dimensions")),
		SortBy: c.QueryParam("sort_by"), SortOrder: c.QueryParam("sort_order"),
	})
	if err != nil {
		return httputil.Error(c, err)
	}
	return c.JSON(http.StatusOK, response)
}

func commaValues(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func attachmentDownloadURL(c *echo.Context, messageID, attachmentID string) string {
	request := c.Request()
	scheme := request.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		if request.TLS != nil { scheme = "https" } else { scheme = "http" }
	}
	host := request.Header.Get("X-Forwarded-Host")
	if host == "" { host = request.Host }
	path := "/emails/" + url.PathEscape(messageID) + "/attachments/" + url.PathEscape(attachmentID) + "/download"
	return scheme + "://" + host + path
}
