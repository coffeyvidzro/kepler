package suppression

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
)

var ErrAlreadyExists = errors.New("suppression already exists")

type Repository struct {
	db      *pgxpool.Pool
	queries *dbsqlc.Queries
	emitter eventEmitter
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db, queries: dbsqlc.New(db)}
}

func NewRepositoryWithEventEmitter(db *pgxpool.Pool, emitter eventEmitter) *Repository {
	return &Repository{db: db, queries: dbsqlc.New(db), emitter: emitter}
}

func (r *Repository) Create(ctx context.Context, teamID uuid.UUID, email string) (Suppression, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Suppression{}, fmt.Errorf("begin suppression creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	queries := r.queries.WithTx(tx)
	row, err := queries.CreateSuppression(ctx, dbsqlc.CreateSuppressionParams{
		TeamID: teamID,
		Email:  email,
		Origin: "manual",
	})
	if isUniqueViolation(err) {
		return Suppression{}, ErrAlreadyExists
	}
	if err != nil {
		return Suppression{}, fmt.Errorf("create suppression: %w", err)
	}
	value := suppressionFromSQLC(row)
	if err := emitSuppressionEvent(ctx, tx, r.emitter, platformevent.TypeSuppressionCreated, value); err != nil {
		return Suppression{}, fmt.Errorf("emit suppression created event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Suppression{}, fmt.Errorf("commit suppression creation: %w", err)
	}
	return value, nil
}

func (r *Repository) CreateBatch(ctx context.Context, teamID uuid.UUID, emails []string) ([]Suppression, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin batch suppression creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := r.queries.WithTx(tx).CreateSuppressions(ctx, dbsqlc.CreateSuppressionsParams{
		TeamID: teamID,
		Emails: emails,
	})
	if isUniqueViolation(err) {
		return nil, ErrAlreadyExists
	}
	if err != nil {
		return nil, fmt.Errorf("create suppressions: %w", err)
	}
	values := suppressionsFromSQLC(rows)
	for _, value := range values {
		if err := emitSuppressionEvent(ctx, tx, r.emitter, platformevent.TypeSuppressionCreated, value); err != nil {
			return nil, fmt.Errorf("emit suppression created event: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit batch suppression creation: %w", err)
	}
	return values, nil
}

func (r *Repository) List(ctx context.Context, teamID uuid.UUID, limit, offset int32) ([]Suppression, error) {
	rows, err := r.queries.ListSuppressions(ctx, dbsqlc.ListSuppressionsParams{
		TeamID:     teamID,
		PageOffset: offset,
		PageLimit:  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list suppressions: %w", err)
	}
	return suppressionsFromSQLC(rows), nil
}

func (r *Repository) ListPage(ctx context.Context, teamID uuid.UUID, limit int32, after, before *uuid.UUID, origin *string) ([]Suppression, error) {
	var (
		rows []dbsqlc.Suppression
		err  error
	)

	switch {
	case after != nil:
		rows, err = r.queries.ListSuppressionsAfter(ctx, dbsqlc.ListSuppressionsAfterParams{
			ScopeTeamID:  teamID,
			FilterOrigin: origin,
			CursorID:     *after,
			PageLimit:    limit,
		})
	case before != nil:
		rows, err = r.queries.ListSuppressionsBefore(ctx, dbsqlc.ListSuppressionsBeforeParams{
			ScopeTeamID:  teamID,
			FilterOrigin: origin,
			CursorID:     *before,
			PageLimit:    limit,
		})
	default:
		rows, err = r.queries.ListSuppressionsFiltered(ctx, dbsqlc.ListSuppressionsFilteredParams{
			TeamID:       teamID,
			FilterOrigin: origin,
			PageLimit:    limit,
		})
	}
	if err != nil {
		return nil, fmt.Errorf("list suppression page: %w", err)
	}
	return suppressionsFromSQLC(rows), nil
}

func (r *Repository) CursorExists(ctx context.Context, teamID, cursorID uuid.UUID) (bool, error) {
	return r.queries.SuppressionCursorExists(ctx, dbsqlc.SuppressionCursorExistsParams{
		CursorID: cursorID,
		TeamID:   teamID,
	})
}

func (r *Repository) GetByID(ctx context.Context, id, teamID uuid.UUID) (Suppression, error) {
	row, err := r.queries.GetSuppressionByID(ctx, dbsqlc.GetSuppressionByIDParams{
		ID:     id,
		TeamID: teamID,
	})
	if err != nil {
		return Suppression{}, err
	}
	return suppressionFromSQLC(row), nil
}

func (r *Repository) GetByEmail(ctx context.Context, email string, teamID uuid.UUID) (Suppression, error) {
	row, err := r.queries.GetSuppressionByEmail(ctx, dbsqlc.GetSuppressionByEmailParams{
		TeamID: teamID,
		Email:  email,
	})
	if err != nil {
		return Suppression{}, err
	}
	return suppressionFromSQLC(row), nil
}

func (r *Repository) DeleteByID(ctx context.Context, id, teamID uuid.UUID) (Suppression, error) {
	return r.delete(ctx, func(queries *dbsqlc.Queries) (dbsqlc.Suppression, error) {
		return queries.DeleteSuppressionByID(ctx, dbsqlc.DeleteSuppressionByIDParams{
			ID:     id,
			TeamID: teamID,
		})
	})
}

func (r *Repository) DeleteByEmail(ctx context.Context, email string, teamID uuid.UUID) (Suppression, error) {
	return r.delete(ctx, func(queries *dbsqlc.Queries) (dbsqlc.Suppression, error) {
		return queries.DeleteSuppressionByEmail(ctx, dbsqlc.DeleteSuppressionByEmailParams{
			TeamID: teamID,
			Email:  email,
		})
	})
}

func (r *Repository) DeleteBatchByIDs(ctx context.Context, teamID uuid.UUID, ids []uuid.UUID) ([]Suppression, error) {
	return r.deleteBatch(ctx, func(queries *dbsqlc.Queries) ([]dbsqlc.Suppression, error) {
		return queries.DeleteSuppressionsByIDs(ctx, dbsqlc.DeleteSuppressionsByIDsParams{
			TeamID: teamID,
			Ids:    ids,
		})
	})
}

func (r *Repository) DeleteBatchByEmails(ctx context.Context, teamID uuid.UUID, emails []string) ([]Suppression, error) {
	return r.deleteBatch(ctx, func(queries *dbsqlc.Queries) ([]dbsqlc.Suppression, error) {
		return queries.DeleteSuppressionsByEmails(ctx, dbsqlc.DeleteSuppressionsByEmailsParams{
			TeamID: teamID,
			Emails: emails,
		})
	})
}

func (r *Repository) delete(ctx context.Context, operation func(*dbsqlc.Queries) (dbsqlc.Suppression, error)) (Suppression, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Suppression{}, fmt.Errorf("begin suppression deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row, err := operation(r.queries.WithTx(tx))
	if err != nil {
		return Suppression{}, err
	}
	value := suppressionFromSQLC(row)
	if err := emitSuppressionEvent(ctx, tx, r.emitter, platformevent.TypeSuppressionDeleted, value); err != nil {
		return Suppression{}, fmt.Errorf("emit suppression deleted event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Suppression{}, fmt.Errorf("commit suppression deletion: %w", err)
	}
	return value, nil
}

func (r *Repository) deleteBatch(ctx context.Context, operation func(*dbsqlc.Queries) ([]dbsqlc.Suppression, error)) ([]Suppression, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin batch suppression deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := operation(r.queries.WithTx(tx))
	if err != nil {
		return nil, fmt.Errorf("delete suppressions: %w", err)
	}
	values := suppressionsFromSQLC(rows)
	for _, value := range values {
		if err := emitSuppressionEvent(ctx, tx, r.emitter, platformevent.TypeSuppressionDeleted, value); err != nil {
			return nil, fmt.Errorf("emit suppression deleted event: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit batch suppression deletion: %w", err)
	}
	return values, nil
}

func suppressionsFromSQLC(rows []dbsqlc.Suppression) []Suppression {
	values := make([]Suppression, 0, len(rows))
	for _, row := range rows {
		values = append(values, suppressionFromSQLC(row))
	}
	return values
}

func suppressionFromSQLC(row dbsqlc.Suppression) Suppression {
	return Suppression{
		ID:        row.ID.String(),
		TeamID:    row.TeamID.String(),
		Email:     row.Email,
		Origin:    row.Origin,
		SourceID:  row.SourceID,
		CreatedAt: row.CreatedAt.Time,
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && strings.EqualFold(pgErr.Code, "23505")
}
