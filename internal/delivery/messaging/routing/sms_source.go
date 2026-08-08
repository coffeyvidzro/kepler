package messagingrouting

import (
	"context"
	"fmt"

	platformrouting "github.com/coffeyvidzro/dugble/server/internal/platform/messaging/routing"
)

type smsDataSource struct{ db DBTX }

func (source smsDataSource) ListCandidates(ctx context.Context, request platformrouting.Request) ([]platformrouting.Candidate, error) {
	if source.db == nil {
		return nil, fmt.Errorf("SMS routing data source is not configured")
	}
	rows, err := source.db.Query(ctx, `
		SELECT sender_id.id, 'sms', sender_id.name, sender_id.normalized_name,
			CASE sender_id.status WHEN 'approved' THEN 'active' WHEN 'rejected' THEN 'failed' WHEN 'inactive' THEN 'disabled' ELSE sender_id.status END,
			sender_id.health_status, sender_id.id, sender_id.team_id, sender_id.id, 'sms',
			CASE WHEN sender_id.disabled_at IS NULL THEN 'active' ELSE 'revoked' END,
			false, (sender_id.disabled_at IS NOT NULL), sender_id.id, sender_id.id,
			COALESCE(sender_id.provider, ''), 'default', '', sender_id.country_code::text,
			CASE sender_id.status WHEN 'approved' THEN 'active' WHEN 'rejected' THEN 'failed' WHEN 'inactive' THEN 'disabled' ELSE sender_id.status END,
			sender_id.provider_whitelisted, sender_id.health_status
		FROM sender_ids AS sender_id
		WHERE sender_id.team_id = $1 AND sender_id.provider IS NOT NULL
		ORDER BY sender_id.provider, sender_id.country_code, sender_id.id
		FOR SHARE OF sender_id
	`, request.TeamID)
	if err != nil {
		return nil, fmt.Errorf("query SMS route candidates: %w", err)
	}
	return scanCandidates(rows)
}
