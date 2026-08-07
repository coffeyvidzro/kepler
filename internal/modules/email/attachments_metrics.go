package email

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

const attachmentDownloadTTL = 15 * time.Minute

type AttachmentMetadata struct {
	Object             string    `json:"object,omitempty"`
	ID                 string    `json:"id"`
	Filename           string    `json:"filename"`
	Size               int       `json:"size"`
	ContentType        string    `json:"content_type"`
	ContentDisposition string    `json:"content_disposition"`
	ContentID          string    `json:"content_id,omitempty"`
	DownloadURL        string    `json:"download_url"`
	ExpiresAt          time.Time `json:"expires_at"`
}

type AttachmentList struct {
	Object  string               `json:"object"`
	HasMore bool                 `json:"has_more"`
	Data    []AttachmentMetadata `json:"data"`
}

type AttachmentDownload struct {
	Filename           string
	ContentType        string
	ContentDisposition string
	Content             []byte
}

type MetricsRequest struct {
	StartDate   string
	EndDate     string
	Timezone    string
	Granularity string
	Metrics     []string
	Dimensions  []string
	SortBy      string
	SortOrder   string
}

type MetricsResponse struct {
	Object      string           `json:"object"`
	StartDate   time.Time        `json:"start_date"`
	EndDate     time.Time        `json:"end_date"`
	Metrics     []string         `json:"metrics"`
	Dimensions  []string         `json:"dimensions"`
	Granularity string           `json:"granularity,omitempty"`
	SortBy      string           `json:"sort_by"`
	SortOrder   string           `json:"sort_order"`
	Totals      map[string]any   `json:"totals"`
	Data        []map[string]any `json:"data,omitempty"`
}

var defaultMetricNames = []string{
	"received", "delivered", "complained", "suppressed", "bounced",
	"bounced_transient", "bounced_permanent", "bounced_undetermined",
	"opened", "clicked", "unsubscribed", "delivery_delayed", "failed", "sent",
	"unique_opened", "unique_clicked", "delivery_rate", "open_rate", "click_rate",
	"bounce_rate", "complaint_rate", "unsubscribe_rate",
}

var metricNameSet = func() map[string]struct{} {
	result := make(map[string]struct{}, len(defaultMetricNames))
	for _, name := range defaultMetricNames {
		result[name] = struct{}{}
	}
	return result
}()

func (s *Service) ListAttachments(ctx context.Context, messageValue string) (AttachmentList, error) {
	message, err := s.Get(ctx, messageValue)
	if err != nil {
		return AttachmentList{}, err
	}
	items := make([]AttachmentMetadata, 0, len(message.Attachments))
	for index, attachment := range message.Attachments {
		metadata, metadataErr := attachmentMetadata(message.ID, index, attachment)
		if metadataErr != nil {
			return AttachmentList{}, metadataErr
		}
		items = append(items, metadata)
	}
	return AttachmentList{Object: "list", HasMore: false, Data: items}, nil
}

func (s *Service) GetAttachment(ctx context.Context, messageValue, attachmentValue string) (AttachmentMetadata, error) {
	list, err := s.ListAttachments(ctx, messageValue)
	if err != nil {
		return AttachmentMetadata{}, err
	}
	for _, item := range list.Data {
		if item.ID == strings.TrimSpace(attachmentValue) {
			item.Object = "attachment"
			return item, nil
		}
	}
	return AttachmentMetadata{}, apperrors.NewNotFound("Email attachment not found")
}

func (s *Service) DownloadAttachment(ctx context.Context, messageValue, attachmentValue string) (AttachmentDownload, error) {
	message, err := s.Get(ctx, messageValue)
	if err != nil {
		return AttachmentDownload{}, err
	}
	for index, attachment := range message.Attachments {
		metadata, metadataErr := attachmentMetadata(message.ID, index, attachment)
		if metadataErr != nil {
			return AttachmentDownload{}, metadataErr
		}
		if metadata.ID != strings.TrimSpace(attachmentValue) {
			continue
		}
		content, decodeErr := base64.StdEncoding.DecodeString(attachment.Content)
		if decodeErr != nil {
			return AttachmentDownload{}, apperrors.NewInternal("Unable to decode email attachment", decodeErr)
		}
		return AttachmentDownload{
			Filename: metadata.Filename, ContentType: metadata.ContentType,
			ContentDisposition: metadata.ContentDisposition, Content: content,
		}, nil
	}
	return AttachmentDownload{}, apperrors.NewNotFound("Email attachment not found")
}

func attachmentMetadata(messageID string, index int, attachment Attachment) (AttachmentMetadata, error) {
	parsedMessageID, err := uuid.Parse(messageID)
	if err != nil {
		return AttachmentMetadata{}, apperrors.NewInternal("Email message id is invalid", err)
	}
	content, err := base64.StdEncoding.DecodeString(attachment.Content)
	if err != nil {
		return AttachmentMetadata{}, apperrors.NewInternal("Stored email attachment is invalid", err)
	}
	contentType := strings.TrimSpace(attachment.ContentType)
	if contentType == "" {
		contentType = mime.TypeByExtension(strings.ToLower(filepath.Ext(attachment.Filename)))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	disposition := "attachment"
	if strings.TrimSpace(attachment.ContentID) != "" {
		disposition = "inline"
	}
	id := uuid.NewSHA1(parsedMessageID, []byte(fmt.Sprintf("attachment:%d", index)))
	return AttachmentMetadata{
		ID: id.String(), Filename: attachment.Filename, Size: len(content), ContentType: contentType,
		ContentDisposition: disposition, ContentID: attachment.ContentID,
		ExpiresAt: time.Now().UTC().Add(attachmentDownloadTTL),
	}, nil
}

func (s *Service) Metrics(ctx context.Context, req MetricsRequest) (MetricsResponse, error) {
	tc, err := requireTenant(ctx, tenant.PermissionEmailRead)
	if err != nil {
		return MetricsResponse{}, err
	}
	start, end, err := metricsRange(req.StartDate, req.EndDate)
	if err != nil {
		return MetricsResponse{}, err
	}
	metrics, err := normalizeMetricNames(req.Metrics)
	if err != nil {
		return MetricsResponse{}, err
	}
	dimensions, err := normalizeDimensions(req.Dimensions)
	if err != nil {
		return MetricsResponse{}, err
	}
	granularity := strings.ToLower(strings.TrimSpace(req.Granularity))
	if granularity == "" {
		granularity = "daily"
	}
	if granularity != "hourly" && granularity != "daily" && granularity != "weekly" {
		return MetricsResponse{}, apperrors.NewBadRequest("granularity must be hourly, daily, or weekly")
	}
	if len(dimensions) > 0 && !(len(dimensions) == 1 && dimensions[0] == "period") {
		return MetricsResponse{}, apperrors.NewBadRequest("Email metrics currently support only the period dimension")
	}

	totals, err := s.repository.EmailMetricTotals(ctx, tc.Scope.TeamID, start, end)
	if err != nil {
		return MetricsResponse{}, apperrors.NewInternal("Unable to retrieve email metrics", err)
	}
	response := MetricsResponse{
		Object: "metrics", StartDate: start, EndDate: end, Metrics: metrics,
		Dimensions: dimensions, Granularity: granularity,
		SortBy: strings.ToLower(strings.TrimSpace(req.SortBy)), SortOrder: strings.ToLower(strings.TrimSpace(req.SortOrder)),
		Totals: selectMetrics(totals, metrics),
	}
	if response.SortBy == "" {
		if len(dimensions) == 1 { response.SortBy = "date" } else { response.SortBy = "sent" }
	}
	if response.SortOrder == "" {
		if response.SortBy == "date" { response.SortOrder = "asc" } else { response.SortOrder = "desc" }
	}
	if response.SortOrder != "asc" && response.SortOrder != "desc" {
		return MetricsResponse{}, apperrors.NewBadRequest("sort_order must be asc or desc")
	}
	if len(dimensions) == 1 {
		rows, rowsErr := s.repository.EmailMetricPeriods(ctx, tc.Scope.TeamID, start, end, granularity)
		if rowsErr != nil {
			return MetricsResponse{}, apperrors.NewInternal("Unable to retrieve email metric periods", rowsErr)
		}
		response.Data = make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			selected := selectMetrics(row.Values, metrics)
			selected["period"] = row.Period
			response.Data = append(response.Data, selected)
		}
		if response.SortOrder == "desc" {
			sort.Slice(response.Data, func(i, j int) bool { return fmt.Sprint(response.Data[i]["period"]) > fmt.Sprint(response.Data[j]["period"]) })
		}
	}
	return response, nil
}

func metricsRange(startValue, endValue string) (time.Time, time.Time, error) {
	now := time.Now().UTC()
	end := now
	var err error
	if strings.TrimSpace(endValue) != "" {
		end, err = parseMetricTime(endValue, false)
		if err != nil { return time.Time{}, time.Time{}, apperrors.NewBadRequest("end_date must be an ISO 8601 date or datetime") }
		if end.After(now) { end = now }
	}
	start := end.AddDate(0, 0, -6)
	if strings.TrimSpace(startValue) != "" {
		start, err = parseMetricTime(startValue, true)
		if err != nil { return time.Time{}, time.Time{}, apperrors.NewBadRequest("start_date must be an ISO 8601 date or datetime") }
	}
	if start.After(end) { return time.Time{}, time.Time{}, apperrors.NewBadRequest("start_date must be on or before end_date") }
	return start.UTC(), end.UTC(), nil
}

func parseMetricTime(value string, startOfDay bool) (time.Time, error) {
	value = strings.TrimSpace(value)
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil { return parsed.UTC(), nil }
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil { return time.Time{}, err }
	if !startOfDay { parsed = parsed.Add(24*time.Hour - time.Nanosecond) }
	return parsed.UTC(), nil
}

func normalizeMetricNames(values []string) ([]string, error) {
	if len(values) == 0 { return append([]string(nil), defaultMetricNames...), nil }
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		name := strings.ToLower(strings.TrimSpace(value))
		if _, ok := metricNameSet[name]; !ok { return nil, apperrors.NewBadRequest("Unsupported email metric: " + name) }
		if _, ok := seen[name]; !ok { result = append(result, name); seen[name] = struct{}{} }
	}
	return result, nil
}

func normalizeDimensions(values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		name := strings.ToLower(strings.TrimSpace(value))
		if name != "period" && name != "domain" && name != "email" { return nil, apperrors.NewBadRequest("Unsupported email metric dimension: " + name) }
		if _, ok := seen[name]; !ok { result = append(result, name); seen[name] = struct{}{} }
	}
	return result, nil
}

func selectMetrics(values map[string]any, metrics []string) map[string]any {
	result := make(map[string]any, len(metrics))
	for _, metric := range metrics {
		if value, ok := values[metric]; ok { result[metric] = value } else { result[metric] = 0 }
	}
	return result
}

type metricPeriod struct {
	Period string
	Values map[string]any
}

func (r *Repository) EmailMetricTotals(ctx context.Context, teamID uuid.UUID, start, end time.Time) (map[string]any, error) {
	return r.emailMetricValues(ctx, teamID, start, end, "")
}

func (r *Repository) EmailMetricPeriods(ctx context.Context, teamID uuid.UUID, start, end time.Time, granularity string) ([]metricPeriod, error) {
	unit := map[string]string{"hourly":"hour", "daily":"day", "weekly":"week"}[granularity]
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT date_trunc('%s', bucket.created_at) AS period,
		       count(*)::bigint AS sent,
		       count(*) FILTER (WHERE bucket.status = 'delivered')::bigint AS delivered,
		       count(*) FILTER (WHERE bucket.status IN ('failed','rejected'))::bigint AS failed,
		       count(*) FILTER (WHERE bucket.status = 'bounced')::bigint AS bounced,
		       count(*) FILTER (WHERE bucket.status = 'complained')::bigint AS complained,
		       count(*) FILTER (WHERE bucket.status = 'delayed')::bigint AS delivery_delayed
		FROM email_messages bucket
		WHERE bucket.team_id = $1 AND bucket.created_at >= $2 AND bucket.created_at <= $3
		GROUP BY 1 ORDER BY 1 ASC`, unit), teamID, start, end)
	if err != nil { return nil, err }
	defer rows.Close()
	result := []metricPeriod{}
	for rows.Next() {
		var period time.Time
		var sent, delivered, failed, bounced, complained, delayed int64
		if err := rows.Scan(&period, &sent, &delivered, &failed, &bounced, &complained, &delayed); err != nil { return nil, err }
		values := metricValues(sent, delivered, failed, bounced, complained, delayed, 0, 0, 0, 0)
		format := "2006-01-02"
		if granularity == "hourly" { format = time.RFC3339 }
		result = append(result, metricPeriod{Period: period.UTC().Format(format), Values: values})
	}
	return result, rows.Err()
}

func (r *Repository) emailMetricValues(ctx context.Context, teamID uuid.UUID, start, end time.Time, _ string) (map[string]any, error) {
	var sent, delivered, failed, bounced, complained, delayed int64
	err := r.db.QueryRow(ctx, `
		SELECT count(*)::bigint,
		       count(*) FILTER (WHERE status = 'delivered')::bigint,
		       count(*) FILTER (WHERE status IN ('failed','rejected'))::bigint,
		       count(*) FILTER (WHERE status = 'bounced')::bigint,
		       count(*) FILTER (WHERE status = 'complained')::bigint,
		       count(*) FILTER (WHERE status = 'delayed')::bigint
		FROM email_messages
		WHERE team_id = $1 AND created_at >= $2 AND created_at <= $3
	`, teamID, start, end).Scan(&sent, &delivered, &failed, &bounced, &complained, &delayed)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) { return nil, err }
	var opened, clicked, uniqueOpened, uniqueClicked int64
	err = r.db.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE event.event_type = 'open')::bigint,
		       count(*) FILTER (WHERE event.event_type = 'click')::bigint,
		       count(DISTINCT event.email_message_id) FILTER (WHERE event.event_type = 'open')::bigint,
		       count(DISTINCT event.email_message_id) FILTER (WHERE event.event_type = 'click')::bigint
		FROM email_provider_events event
		JOIN email_messages message ON message.id = event.email_message_id
		WHERE message.team_id = $1 AND event.occurred_at >= $2 AND event.occurred_at <= $3
	`, teamID, start, end).Scan(&opened, &clicked, &uniqueOpened, &uniqueClicked)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) { return nil, err }
	return metricValues(sent, delivered, failed, bounced, complained, delayed, opened, clicked, uniqueOpened, uniqueClicked), nil
}

func metricValues(sent, delivered, failed, bounced, complained, delayed, opened, clicked, uniqueOpened, uniqueClicked int64) map[string]any {
	values := map[string]any{
		"received": int64(0), "delivered": delivered, "complained": complained, "suppressed": int64(0),
		"bounced": bounced, "bounced_transient": int64(0), "bounced_permanent": bounced,
		"bounced_undetermined": int64(0), "opened": opened, "clicked": clicked,
		"unsubscribed": int64(0), "delivery_delayed": delayed, "failed": failed, "sent": sent,
		"unique_opened": uniqueOpened, "unique_clicked": uniqueClicked,
	}
	values["delivery_rate"] = percentage(delivered, sent)
	values["open_rate"] = percentage(uniqueOpened, delivered)
	values["click_rate"] = percentage(uniqueClicked, delivered)
	values["bounce_rate"] = percentage(bounced, sent)
	values["complaint_rate"] = percentage(complained, delivered)
	values["unsubscribe_rate"] = float64(0)
	return values
}

func percentage(value, total int64) float64 {
	if total == 0 { return 0 }
	return float64(value) * 100 / float64(total)
}
