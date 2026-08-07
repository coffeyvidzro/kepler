package broadcast

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type MaterializationResult struct {
	BroadcastID    uuid.UUID
	TeamID         uuid.UUID
	AudienceCount  int64
	EligibleCount  int64
	ExcludedCount  int64
}

// MaterializeNextQueuedRecipients freezes the next queued Broadcast audience.
// The Broadcast row is locked until recipients, counters, and the completion
// marker are committed, making the operation safe to retry across workers.
func (r *Repository) MaterializeNextQueuedRecipients(ctx context.Context) (MaterializationResult, bool, error) {
	if r == nil || r.db == nil {
		return MaterializationResult{}, false, errors.New("broadcast repository is not configured")
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return MaterializationResult{}, false, fmt.Errorf("begin recipient materialization: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var broadcastID, teamID, segmentID uuid.UUID
	var topicID *uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT id, team_id, segment_id, topic_id
		FROM broadcasts
		WHERE status = 'queued'
		  AND recipients_materialized_at IS NULL
		  AND deleted_at IS NULL
		ORDER BY queued_at, id
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`).Scan(&broadcastID, &teamID, &segmentID, &topicID)
	if errors.Is(err, pgx.ErrNoRows) {
		return MaterializationResult{}, false, nil
	}
	if err != nil {
		return MaterializationResult{}, false, fmt.Errorf("claim queued broadcast: %w", err)
	}

	_, err = tx.Exec(ctx, `
		WITH candidates AS (
			SELECT
				c.id AS contact_id,
				c.email,
				lower(btrim(c.email)) AS normalized_email,
				c.first_name,
				c.last_name,
				jsonb_build_object(
					'id', c.id,
					'email', c.email,
					'first_name', c.first_name,
					'last_name', c.last_name,
					'properties', COALESCE(properties.values, '{}'::jsonb)
				) AS contact_snapshot,
				CASE
					WHEN c.email !~* '^[A-Z0-9.!#$%&''*+/=?^_` + "`" + `{|}~-]+@[A-Z0-9](?:[A-Z0-9-]{0,61}[A-Z0-9])?(?:\\.[A-Z0-9](?:[A-Z0-9-]{0,61}[A-Z0-9])?)+$' THEN 'invalid_email'
					WHEN c.unsubscribed THEN 'global_unsubscribe'
					WHEN EXISTS (
						SELECT 1 FROM suppressions s
						WHERE s.team_id = c.team_id AND lower(s.email) = lower(c.email)
					) THEN 'suppressed'
					WHEN $4::uuid IS NOT NULL AND COALESCE(cts.subscription, t.default_subscription) = 'opt_out' THEN 'topic_unsubscribed'
					ELSE NULL
				END AS exclusion_reason
			FROM contact_segments cs
			JOIN contacts c
			  ON c.id = cs.contact_id AND c.team_id = cs.team_id
			LEFT JOIN topics t
			  ON t.id = $4 AND t.team_id = c.team_id
			LEFT JOIN contact_topic_subscriptions cts
			  ON cts.contact_id = c.id AND cts.topic_id = $4 AND cts.team_id = c.team_id
			LEFT JOIN LATERAL (
				SELECT jsonb_object_agg(
					cp.key,
					CASE cpv.value_type
						WHEN 'string' THEN to_jsonb(cpv.string_value)
						WHEN 'number' THEN to_jsonb(cpv.number_value)
					END
				) AS values
				FROM contact_property_values cpv
				JOIN contact_properties cp
				  ON cp.id = cpv.contact_property_id AND cp.team_id = cpv.team_id
				WHERE cpv.contact_id = c.id AND cpv.team_id = c.team_id
			) properties ON true
			WHERE cs.team_id = $2 AND cs.segment_id = $3
		)
		INSERT INTO broadcast_recipients (
			team_id, broadcast_id, contact_id, email, normalized_email,
			first_name, last_name, contact_snapshot, status, exclusion_reason
		)
		SELECT
			$2, $1, contact_id, email, normalized_email,
			first_name, last_name, contact_snapshot,
			CASE WHEN exclusion_reason IS NULL THEN 'pending' ELSE 'excluded' END,
			exclusion_reason
		FROM candidates
		ON CONFLICT DO NOTHING
	`, broadcastID, teamID, segmentID, topicID)
	if err != nil {
		return MaterializationResult{}, false, fmt.Errorf("insert broadcast recipients: %w", err)
	}

	var result MaterializationResult
	result.BroadcastID = broadcastID
	result.TeamID = teamID
	err = tx.QueryRow(ctx, `
		WITH counts AS (
			SELECT
				count(*) AS audience_count,
				count(*) FILTER (WHERE status = 'pending') AS eligible_count,
				count(*) FILTER (WHERE status = 'excluded') AS excluded_count
			FROM broadcast_recipients
			WHERE broadcast_id = $1 AND team_id = $2
		)
		UPDATE broadcasts AS b
		SET audience_count = counts.audience_count,
			eligible_count = counts.eligible_count,
			suppressed_count = counts.excluded_count,
			failed_count = 0,
			recipients_materialized_at = now(),
			revision = revision + 1,
			updated_at = now()
		FROM counts
		WHERE b.id = $1 AND b.team_id = $2
		RETURNING counts.audience_count, counts.eligible_count, counts.excluded_count
	`, broadcastID, teamID).Scan(&result.AudienceCount, &result.EligibleCount, &result.ExcludedCount)
	if err != nil {
		return MaterializationResult{}, false, fmt.Errorf("complete recipient materialization: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return MaterializationResult{}, false, fmt.Errorf("commit recipient materialization: %w", err)
	}
	return result, true, nil
}
