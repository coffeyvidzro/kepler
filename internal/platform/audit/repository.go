package audit

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
)

type Repository struct{ queries *dbsqlc.Queries }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{queries: dbsqlc.New(db)} }

func (r *Repository) Record(ctx context.Context, entry Entry) error {
	metadata, err := json.Marshal(entry.Metadata)
	if err != nil {
		return err
	}
	_, err = r.queries.CreateAuditEvent(ctx, dbsqlc.CreateAuditEventParams{
		TeamID: uuidPointer(entry.TeamID), ActorType: entry.ActorType,
		ActorUserID: uuidPointer(entry.ActorUserID), ActorSessionID: stringPointer(entry.ActorSessionID),
		ActorTokenID: uuidPointer(entry.ActorTokenID), Action: entry.Action,
		ResourceType: entry.ResourceType, ResourceID: entry.ResourceID, Outcome: entry.Outcome,
		Metadata: metadata, RequestID: stringPointer(entry.Request.RequestID),
		IpAddress: stringPointer(entry.Request.IPAddress), UserAgent: stringPointer(entry.Request.UserAgent),
	})
	return err
}

func (r *Repository) ListTeam(ctx context.Context, teamID uuid.UUID, beforeID *uuid.UUID, pageSize int32) ([]Entry, error) {
	rows, err := r.queries.ListTeamAuditEvents(ctx, dbsqlc.ListTeamAuditEventsParams{TeamID: &teamID, BeforeID: beforeID, PageSize: pageSize})
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(rows))
	for _, row := range rows {
		entry := Entry{ID: row.ID, ActorType: row.ActorType, Action: row.Action, ResourceType: row.ResourceType, ResourceID: row.ResourceID, Outcome: row.Outcome, CreatedAt: row.CreatedAt.Time}
		if row.TeamID != nil {
			entry.TeamID = *row.TeamID
		}
		if row.ActorUserID != nil {
			entry.ActorUserID = *row.ActorUserID
		}
		if row.ActorSessionID != nil {
			entry.ActorSessionID = *row.ActorSessionID
		}
		if row.ActorTokenID != nil {
			entry.ActorTokenID = *row.ActorTokenID
		}
		if err := json.Unmarshal(row.Metadata, &entry.Metadata); err != nil {
			return nil, err
		}
		if row.RequestID != nil {
			entry.Request.RequestID = *row.RequestID
		}
		if row.IpAddress != nil {
			entry.Request.IPAddress = *row.IpAddress
		}
		if row.UserAgent != nil {
			entry.Request.UserAgent = *row.UserAgent
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func uuidPointer(value uuid.UUID) *uuid.UUID {
	if value == uuid.Nil {
		return nil
	}
	return &value
}
func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
