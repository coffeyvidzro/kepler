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

	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
)

var ErrAlreadyExists = errors.New("suppression already exists")

type Repository struct {
	db      *pgxpool.Pool
	emitter eventEmitter
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func NewRepositoryWithEventEmitter(db *pgxpool.Pool, emitter eventEmitter) *Repository {
	return &Repository{db: db, emitter: emitter}
}

func (r *Repository) Create(ctx context.Context, teamID uuid.UUID, email string) (Suppression, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Suppression{}, fmt.Errorf("begin suppression creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	value, err := scanSuppression(tx.QueryRow(ctx, `
		INSERT INTO suppressions (team_id, email, origin)
		VALUES ($1, $2, 'manual')
		RETURNING id, team_id, email, origin, source_id, created_at
	`, teamID, email))
	if isUniqueViolation(err) {
		return Suppression{}, ErrAlreadyExists
	}
	if err != nil {
		return Suppression{}, fmt.Errorf("create suppression: %w", err)
	}
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

	rows, err := tx.Query(ctx, `
		INSERT INTO suppressions (team_id, email, origin)
		SELECT $1, lower(batch_email.email), 'manual'
		FROM unnest($2::text[]) WITH ORDINALITY AS batch_email(email, position)
		ORDER BY batch_email.position
		RETURNING id, team_id, email, origin, source_id, created_at
	`, teamID, emails)
	if isUniqueViolation(err) {
		return nil, ErrAlreadyExists
	}
	if err != nil {
		return nil, fmt.Errorf("create suppressions: %w", err)
	}
	values, err := scanSuppressions(rows)
	if isUniqueViolation(err) {
		return nil, ErrAlreadyExists
	}
	if err != nil {
		return nil, fmt.Errorf("scan created suppressions: %w", err)
	}
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
	rows, err := r.db.Query(ctx, `
		SELECT id, team_id, email, origin, source_id, created_at
		FROM suppressions WHERE team_id = $1
		ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3
	`, teamID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list suppressions: %w", err)
	}
	values, err := scanSuppressions(rows)
	if err != nil {
		return nil, fmt.Errorf("scan suppressions: %w", err)
	}
	return values, nil
}

func (r *Repository) ListPage(ctx context.Context, teamID uuid.UUID, limit int32, after, before *uuid.UUID, origin *string) ([]Suppression, error) {
	query := `SELECT id, team_id, email, origin, source_id, created_at FROM suppressions WHERE team_id = $1`
	args := []any{teamID}
	if origin != nil {
		args = append(args, *origin)
		query += fmt.Sprintf(` AND origin = $%d`, len(args))
	}
	if after != nil {
		args = append(args, *after)
		query += fmt.Sprintf(` AND (created_at, id) < (SELECT created_at, id FROM suppressions WHERE id = $%d AND team_id = $1)`, len(args))
	}
	if before != nil {
		args = append(args, *before)
		query += fmt.Sprintf(` AND (created_at, id) > (SELECT created_at, id FROM suppressions WHERE id = $%d AND team_id = $1)`, len(args))
	}
	if before != nil {
		query += ` ORDER BY created_at ASC, id ASC`
	} else {
		query += ` ORDER BY created_at DESC, id DESC`
	}
	args = append(args, limit)
	query += fmt.Sprintf(` LIMIT $%d`, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list suppression page: %w", err)
	}
	values, err := scanSuppressions(rows)
	if err != nil {
		return nil, fmt.Errorf("scan suppression page: %w", err)
	}
	return values, nil
}

func (r *Repository) CursorExists(ctx context.Context, teamID, cursorID uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM suppressions WHERE id = $1 AND team_id = $2)`, cursorID, teamID).Scan(&exists)
	return exists, err
}

func (r *Repository) GetByID(ctx context.Context, id, teamID uuid.UUID) (Suppression, error) {
	return scanSuppression(r.db.QueryRow(ctx, `SELECT id, team_id, email, origin, source_id, created_at FROM suppressions WHERE id = $1 AND team_id = $2`, id, teamID))
}

func (r *Repository) GetByEmail(ctx context.Context, email string, teamID uuid.UUID) (Suppression, error) {
	return scanSuppression(r.db.QueryRow(ctx, `SELECT id, team_id, email, origin, source_id, created_at FROM suppressions WHERE team_id = $1 AND lower(email) = lower($2)`, teamID, email))
}

func (r *Repository) DeleteByID(ctx context.Context, id, teamID uuid.UUID) (Suppression, error) {
	return r.delete(ctx, `DELETE FROM suppressions WHERE id = $1 AND team_id = $2 RETURNING id, team_id, email, origin, source_id, created_at`, id, teamID)
}

func (r *Repository) DeleteByEmail(ctx context.Context, email string, teamID uuid.UUID) (Suppression, error) {
	return r.delete(ctx, `DELETE FROM suppressions WHERE team_id = $1 AND lower(email) = lower($2) RETURNING id, team_id, email, origin, source_id, created_at`, teamID, email)
}

func (r *Repository) DeleteBatchByIDs(ctx context.Context, teamID uuid.UUID, ids []uuid.UUID) ([]Suppression, error) {
	return r.deleteBatch(ctx, `
		DELETE FROM suppressions
		WHERE team_id = $1 AND id = ANY($2::uuid[])
		RETURNING id, team_id, email, origin, source_id, created_at
	`, teamID, ids)
}

func (r *Repository) DeleteBatchByEmails(ctx context.Context, teamID uuid.UUID, emails []string) ([]Suppression, error) {
	return r.deleteBatch(ctx, `
		DELETE FROM suppressions
		WHERE team_id = $1 AND lower(email) = ANY($2::text[])
		RETURNING id, team_id, email, origin, source_id, created_at
	`, teamID, emails)
}

func (r *Repository) delete(ctx context.Context, query string, args ...any) (Suppression, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Suppression{}, fmt.Errorf("begin suppression deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	value, err := scanSuppression(tx.QueryRow(ctx, query, args...))
	if err != nil {
		return Suppression{}, err
	}
	if err := emitSuppressionEvent(ctx, tx, r.emitter, platformevent.TypeSuppressionDeleted, value); err != nil {
		return Suppression{}, fmt.Errorf("emit suppression deleted event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Suppression{}, fmt.Errorf("commit suppression deletion: %w", err)
	}
	return value, nil
}

func (r *Repository) deleteBatch(ctx context.Context, query string, args ...any) ([]Suppression, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin batch suppression deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("delete suppressions: %w", err)
	}
	values, err := scanSuppressions(rows)
	if err != nil {
		return nil, fmt.Errorf("scan deleted suppressions: %w", err)
	}
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

type suppressionScanner interface {
	Scan(dest ...any) error
}

func scanSuppression(row suppressionScanner) (Suppression, error) {
	var value Suppression
	err := row.Scan(&value.ID, &value.TeamID, &value.Email, &value.Origin, &value.SourceID, &value.CreatedAt)
	return value, err
}

func scanSuppressions(rows pgx.Rows) ([]Suppression, error) {
	defer rows.Close()
	values := make([]Suppression, 0)
	for rows.Next() {
		value, err := scanSuppression(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && strings.EqualFold(pgErr.Code, "23505")
}
