package dashboard

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Stats(ctx context.Context) (Stats, error) {
	var stats Stats
	if err := r.db.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM users),
			(SELECT count(*) FROM teams),
			(SELECT count(*) FROM sms_messages WHERE created_at >= date_trunc('day', now())),
			(SELECT count(*) FROM sms_messages WHERE created_at >= now() - interval '24 hours' AND status IN ('failed', 'undelivered', 'rejected', 'expired')),
			(SELECT count(*) FROM sender_ids WHERE status = 'pending'),
			(SELECT count(*) FROM sender_domains WHERE status = 'pending')
	`).Scan(&stats.Users, &stats.Teams, &stats.SMSToday, &stats.FailedSMS24Hours, &stats.PendingSenderIDs, &stats.PendingDomains); err != nil {
		return Stats{}, fmt.Errorf("load dashboard stats: %w", err)
	}

	return stats, nil
}
