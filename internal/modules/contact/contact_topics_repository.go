package contact

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
)

var (
	ErrContactTopicCursorNotFound = errors.New("contact topic cursor not found")
	ErrTopicNotFound              = errors.New("topic not found")
)

func (r *Repository) ListTopics(ctx context.Context, identifier string, teamID uuid.UUID, req ListContactTopicsRequest) ([]ContactTopic, bool, string, error) {
	contactID, err := r.resolveContactID(ctx, identifier, teamID)
	if err != nil {
		return nil, false, "", err
	}
	queries := dbsqlc.New(r.db)
	limit := req.Limit + 1
	var topics []ContactTopic

	switch {
	case req.After != "":
		cursorID, parseErr := uuid.Parse(req.After)
		if parseErr != nil {
			return nil, false, "", ErrContactTopicCursorNotFound
		}
		if err := ensureContactTopicCursor(ctx, queries, teamID, cursorID); err != nil {
			return nil, false, "", err
		}
		rows, queryErr := queries.ListContactTopicsAfter(ctx, dbsqlc.ListContactTopicsAfterParams{
			ContactID: contactID,
			ScopeTeamID: teamID,
			CursorID: cursorID,
			PageLimit: limit,
		})
		if queryErr != nil {
			return nil, false, "", fmt.Errorf("list contact topics after cursor: %w", queryErr)
		}
		topics = make([]ContactTopic, 0, len(rows))
		for _, row := range rows {
			topics = append(topics, ContactTopic{ID: row.ID.String(), Name: row.Name, Description: row.Description, Subscription: row.Subscription})
		}
	case req.Before != "":
		cursorID, parseErr := uuid.Parse(req.Before)
		if parseErr != nil {
			return nil, false, "", ErrContactTopicCursorNotFound
		}
		if err := ensureContactTopicCursor(ctx, queries, teamID, cursorID); err != nil {
			return nil, false, "", err
		}
		rows, queryErr := queries.ListContactTopicsBefore(ctx, dbsqlc.ListContactTopicsBeforeParams{
			ContactID: contactID,
			ScopeTeamID: teamID,
			CursorID: cursorID,
			PageLimit: limit,
		})
		if queryErr != nil {
			return nil, false, "", fmt.Errorf("list contact topics before cursor: %w", queryErr)
		}
		topics = make([]ContactTopic, 0, len(rows))
		for _, row := range rows {
			topics = append(topics, ContactTopic{ID: row.ID.String(), Name: row.Name, Description: row.Description, Subscription: row.Subscription})
		}
	default:
		rows, queryErr := queries.ListContactTopics(ctx, dbsqlc.ListContactTopicsParams{
			ContactID: contactID,
			TeamID: teamID,
			PageLimit: limit,
		})
		if queryErr != nil {
			return nil, false, "", fmt.Errorf("list contact topics: %w", queryErr)
		}
		topics = make([]ContactTopic, 0, len(rows))
		for _, row := range rows {
			topics = append(topics, ContactTopic{ID: row.ID.String(), Name: row.Name, Description: row.Description, Subscription: row.Subscription})
		}
	}

	hasMore := len(topics) > int(req.Limit)
	if hasMore {
		topics = topics[:req.Limit]
	}
	if req.Before != "" {
		slices.Reverse(topics)
	}
	return topics, hasMore, contactID.String(), nil
}

func (r *Repository) UpdateTopics(ctx context.Context, identifier string, teamID uuid.UUID, updates UpdateContactTopicsRequest) (string, error) {
	contactID, err := r.resolveContactID(ctx, identifier, teamID)
	if err != nil {
		return "", err
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", fmt.Errorf("begin contact topic update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbsqlc.New(tx)

	for _, update := range updates {
		topicID, parseErr := uuid.Parse(update.ID)
		if parseErr != nil {
			return "", ErrTopicNotFound
		}
		if _, getErr := queries.GetTopic(ctx, dbsqlc.GetTopicParams{ID: topicID, TeamID: teamID}); errors.Is(getErr, pgx.ErrNoRows) {
			return "", ErrTopicNotFound
		} else if getErr != nil {
			return "", fmt.Errorf("validate contact topic: %w", getErr)
		}
		if _, upsertErr := queries.UpsertContactTopicSubscription(ctx, dbsqlc.UpsertContactTopicSubscriptionParams{
			TeamID: teamID,
			ContactID: contactID,
			TopicID: topicID,
			Subscription: update.Subscription,
		}); upsertErr != nil {
			return "", fmt.Errorf("update contact topic subscription: %w", upsertErr)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit contact topic update: %w", err)
	}
	return contactID.String(), nil
}

func (r *Repository) resolveContactID(ctx context.Context, identifier string, teamID uuid.UUID) (uuid.UUID, error) {
	queries := dbsqlc.New(r.db)
	if id, err := uuid.Parse(strings.TrimSpace(identifier)); err == nil {
		contact, getErr := queries.GetContact(ctx, dbsqlc.GetContactParams{ID: id, TeamID: teamID})
		if getErr != nil {
			return uuid.Nil, getErr
		}
		return contact.ID, nil
	}
	contact, err := queries.GetContactByEmail(ctx, dbsqlc.GetContactByEmailParams{Email: strings.TrimSpace(identifier), TeamID: teamID})
	if err != nil {
		return uuid.Nil, err
	}
	return contact.ID, nil
}

func ensureContactTopicCursor(ctx context.Context, queries *dbsqlc.Queries, teamID, cursorID uuid.UUID) error {
	exists, err := queries.ContactTopicCursorExists(ctx, dbsqlc.ContactTopicCursorExistsParams{CursorID: cursorID, TeamID: teamID})
	if err != nil {
		return fmt.Errorf("validate contact topic cursor: %w", err)
	}
	if !exists {
		return ErrContactTopicCursorNotFound
	}
	return nil
}
