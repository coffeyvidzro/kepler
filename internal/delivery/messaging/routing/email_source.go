package messagingrouting

import (
	"context"
	"fmt"

	platformrouting "github.com/coffeyvidzro/dugble/server/internal/platform/messaging/routing"
)

type emailDataSource struct{ db DBTX }

func (source emailDataSource) ListCandidates(ctx context.Context, request platformrouting.Request) ([]platformrouting.Candidate, error) {
	if source.db == nil {
		return nil, fmt.Errorf("email routing data source is not configured")
	}
	rows, err := source.db.Query(ctx, `
		SELECT domain.id, 'email', domain.name, domain.normalized_name,
			CASE domain.status WHEN 'verified' THEN 'active' WHEN 'failed' THEN 'failed' WHEN 'disabled' THEN 'disabled' ELSE 'pending' END,
			domain.health_status, domain.id, domain.team_id, domain.id, 'email',
			CASE WHEN domain.disabled_at IS NULL AND domain.sending_enabled THEN 'active' ELSE 'revoked' END,
			false, (domain.disabled_at IS NOT NULL OR NOT domain.sending_enabled), domain.id, domain.id,
			domain.provider, domain.provider_account, domain.provider_region, '',
			CASE domain.status WHEN 'verified' THEN 'active' WHEN 'failed' THEN 'failed' WHEN 'disabled' THEN 'disabled' ELSE 'pending' END,
			(domain.status = 'verified'), domain.health_status
		FROM domains AS domain
		WHERE domain.team_id = $1
		ORDER BY domain.provider, domain.provider_account, domain.provider_region, domain.id
		FOR SHARE OF domain
	`, request.TeamID)
	if err != nil {
		return nil, fmt.Errorf("query email route candidates: %w", err)
	}
	return scanCandidates(rows)
}
