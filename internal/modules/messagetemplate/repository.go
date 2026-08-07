package messagetemplate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound        = errors.New("message template not found")
	ErrVersionNotFound = errors.New("message template version not found")
	ErrAliasConflict   = errors.New("message template alias already exists")
	ErrVersionConflict = errors.New("message template version conflict")
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) Create(ctx context.Context, teamID uuid.UUID, req CreateRequest) (Template, Version, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Template{}, Version{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var template Template
	err = tx.QueryRow(ctx, `INSERT INTO message_templates (team_id,name,alias) VALUES ($1,$2,$3)
		RETURNING id,team_id,name,alias,current_version_id,published_version_id,published_at,created_at,updated_at`, teamID, req.Name, req.Alias).
		Scan(&template.ID, &template.TeamID, &template.Name, &template.Alias, &template.CurrentVersionID, &template.PublishedVersionID, &template.PublishedAt, &template.CreatedAt, &template.UpdatedAt)
	if err != nil {
		return Template{}, Version{}, mapWriteError(err)
	}
	templateID := uuid.MustParse(template.ID)
	variables, err := encodeVariables(req.Variables)
	if err != nil {
		return Template{}, Version{}, err
	}
	version, err := insertVersion(ctx, tx, teamID, templateID, 1, req.FromEmail, req.FromName, req.ReplyTo, req.Subject, req.HTML, req.Text, variables, nil, nil)
	if err != nil {
		return Template{}, Version{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE message_templates SET current_version_id=$1,next_version_number=2,updated_at=now() WHERE id=$2 AND team_id=$3`, uuid.MustParse(version.ID), templateID, teamID)
	if err != nil {
		return Template{}, Version{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Template{}, Version{}, err
	}
	template.CurrentVersionID = &version.ID
	return template, version, nil
}

func (r *Repository) List(ctx context.Context, teamID uuid.UUID, limit, offset int32) ([]Template, error) {
	rows, err := r.db.Query(ctx, `SELECT id,team_id,name,alias,current_version_id,published_version_id,published_at,created_at,updated_at FROM message_templates WHERE team_id=$1 AND deleted_at IS NULL ORDER BY created_at DESC,id DESC LIMIT $2 OFFSET $3`, teamID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Template{}
	for rows.Next() {
		var value Template
		if err := rows.Scan(&value.ID, &value.TeamID, &value.Name, &value.Alias, &value.CurrentVersionID, &value.PublishedVersionID, &value.PublishedAt, &value.CreatedAt, &value.UpdatedAt); err != nil {
			return nil, err
		}
		value.HasUnpublishedChanges = value.CurrentVersionID != nil && (value.PublishedVersionID == nil || *value.CurrentVersionID != *value.PublishedVersionID)
		result = append(result, value)
	}
	return result, rows.Err()
}

func (r *Repository) Resolve(ctx context.Context, teamID uuid.UUID, identifier string) (Template, error) {
	var value Template
	id, idErr := uuid.Parse(identifier)
	query := `SELECT id,team_id,name,alias,current_version_id,published_version_id,published_at,created_at,updated_at FROM message_templates WHERE team_id=$1 AND deleted_at IS NULL AND lower(alias)=lower($2)`
	args := []any{teamID, identifier}
	if idErr == nil {
		query = `SELECT id,team_id,name,alias,current_version_id,published_version_id,published_at,created_at,updated_at FROM message_templates WHERE team_id=$1 AND id=$2 AND deleted_at IS NULL`
		args = []any{teamID, id}
	}
	err := r.db.QueryRow(ctx, query, args...).Scan(&value.ID, &value.TeamID, &value.Name, &value.Alias, &value.CurrentVersionID, &value.PublishedVersionID, &value.PublishedAt, &value.CreatedAt, &value.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Template{}, ErrNotFound
	}
	if err != nil {
		return Template{}, err
	}
	value.HasUnpublishedChanges = value.CurrentVersionID != nil && (value.PublishedVersionID == nil || *value.CurrentVersionID != *value.PublishedVersionID)
	return value, nil
}

func (r *Repository) GetVersion(ctx context.Context, teamID, templateID, versionID uuid.UUID) (Version, error) {
	var value Version
	var variables []byte
	err := r.db.QueryRow(ctx, `SELECT id,team_id,template_id,version_number,from_email,from_name,reply_to_email,subject,html_body,text_body,variables,based_on_version_id,change_note,created_at FROM message_template_versions WHERE id=$1 AND template_id=$2 AND team_id=$3`, versionID, templateID, teamID).
		Scan(&value.ID, &value.TeamID, &value.TemplateID, &value.VersionNumber, &value.FromEmail, &value.FromName, &value.ReplyToEmail, &value.Subject, &value.HTML, &value.Text, &variables, &value.BasedOnVersionID, &value.ChangeNote, &value.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, ErrVersionNotFound
	}
	if err != nil {
		return Version{}, err
	}
	if err := json.Unmarshal(variables, &value.Variables); err != nil {
		return Version{}, err
	}
	return value, nil
}

func (r *Repository) ListVersions(ctx context.Context, teamID, templateID uuid.UUID, limit, offset int32) ([]Version, error) {
	rows, err := r.db.Query(ctx, `SELECT id,team_id,template_id,version_number,from_email,from_name,reply_to_email,subject,html_body,text_body,variables,based_on_version_id,change_note,created_at FROM message_template_versions WHERE template_id=$1 AND team_id=$2 ORDER BY version_number DESC LIMIT $3 OFFSET $4`, templateID, teamID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Version{}
	for rows.Next() {
		var value Version
		var variables []byte
		if err := rows.Scan(&value.ID, &value.TeamID, &value.TemplateID, &value.VersionNumber, &value.FromEmail, &value.FromName, &value.ReplyToEmail, &value.Subject, &value.HTML, &value.Text, &variables, &value.BasedOnVersionID, &value.ChangeNote, &value.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(variables, &value.Variables); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (r *Repository) Update(ctx context.Context, teamID uuid.UUID, template Template, base Version, req UpdateRequest) (Template, Version, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Template{}, Version{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var currentID *uuid.UUID
	var next int32
	err = tx.QueryRow(ctx, `SELECT current_version_id,next_version_number FROM message_templates WHERE id=$1 AND team_id=$2 AND deleted_at IS NULL FOR UPDATE`, uuid.MustParse(template.ID), teamID).Scan(&currentID, &next)
	if errors.Is(err, pgx.ErrNoRows) {
		return Template{}, Version{}, ErrNotFound
	}
	if err != nil {
		return Template{}, Version{}, err
	}
	if currentID == nil || currentID.String() != base.ID {
		return Template{}, Version{}, ErrVersionConflict
	}
	name, alias := template.Name, template.Alias
	if req.Name != nil {
		name = *req.Name
	}
	if req.Alias != nil {
		alias = *req.Alias
	}
	_, err = tx.Exec(ctx, `UPDATE message_templates SET name=$1,alias=$2,updated_at=now() WHERE id=$3 AND team_id=$4`, name, alias, uuid.MustParse(template.ID), teamID)
	if err != nil {
		return Template{}, Version{}, mapWriteError(err)
	}
	fromEmail, fromName, replyTo, subject, htmlBody, textBody, variables := base.FromEmail, base.FromName, base.ReplyToEmail, base.Subject, base.HTML, base.Text, base.Variables
	if req.FromEmail != nil {
		fromEmail = *req.FromEmail
	}
	if req.FromName != nil {
		fromName = *req.FromName
	}
	if req.ReplyTo != nil {
		replyTo = *req.ReplyTo
	}
	if req.Subject != nil {
		subject = *req.Subject
	}
	if req.HTML != nil {
		htmlBody = *req.HTML
	}
	if req.Text != nil {
		textBody = *req.Text
	}
	if req.Variables != nil {
		variables = *req.Variables
	}
	encoded, err := encodeVariables(variables)
	if err != nil {
		return Template{}, Version{}, err
	}
	based := uuid.MustParse(base.ID)
	version, err := insertVersion(ctx, tx, teamID, uuid.MustParse(template.ID), next, fromEmail, fromName, replyTo, subject, htmlBody, textBody, encoded, &based, req.ChangeNote)
	if err != nil {
		return Template{}, Version{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE message_templates SET current_version_id=$1,next_version_number=next_version_number+1,updated_at=now() WHERE id=$2 AND team_id=$3`, uuid.MustParse(version.ID), uuid.MustParse(template.ID), teamID)
	if err != nil {
		return Template{}, Version{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Template{}, Version{}, err
	}
	template.Name = name
	template.Alias = alias
	template.CurrentVersionID = &version.ID
	template.HasUnpublishedChanges = true
	return template, version, nil
}

func (r *Repository) Publish(ctx context.Context, teamID uuid.UUID, templateID, versionID uuid.UUID) (Template, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Template{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var exists bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM message_template_versions WHERE id=$1 AND template_id=$2 AND team_id=$3)`, versionID, templateID, teamID).Scan(&exists)
	if err != nil || !exists {
		if err == nil {
			err = ErrVersionNotFound
		}
		return Template{}, err
	}
	var value Template
	err = tx.QueryRow(ctx, `UPDATE message_templates SET published_version_id=$1,published_at=now(),updated_at=now() WHERE id=$2 AND team_id=$3 AND deleted_at IS NULL RETURNING id,team_id,name,alias,current_version_id,published_version_id,published_at,created_at,updated_at`, versionID, templateID, teamID).Scan(&value.ID, &value.TeamID, &value.Name, &value.Alias, &value.CurrentVersionID, &value.PublishedVersionID, &value.PublishedAt, &value.CreatedAt, &value.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Template{}, ErrNotFound
	}
	if err != nil {
		return Template{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO message_template_publications(team_id,template_id,version_id) VALUES($1,$2,$3)`, teamID, templateID, versionID)
	if err != nil {
		return Template{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Template{}, err
	}
	value.HasUnpublishedChanges = value.CurrentVersionID != nil && *value.CurrentVersionID != versionID.String()
	return value, nil
}

func (r *Repository) Delete(ctx context.Context, teamID, templateID uuid.UUID) (Template, error) {
	var value Template
	err := r.db.QueryRow(ctx, `UPDATE message_templates SET deleted_at=now(),updated_at=now() WHERE id=$1 AND team_id=$2 AND deleted_at IS NULL RETURNING id,team_id,name,alias,current_version_id,published_version_id,published_at,created_at,updated_at`, templateID, teamID).Scan(&value.ID, &value.TeamID, &value.Name, &value.Alias, &value.CurrentVersionID, &value.PublishedVersionID, &value.PublishedAt, &value.CreatedAt, &value.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Template{}, ErrNotFound
	}
	return value, err
}

func insertVersion(ctx context.Context, tx pgx.Tx, teamID, templateID uuid.UUID, number int32, fromEmail, fromName, replyTo *string, subject, htmlBody string, textBody *string, variables []byte, basedOn *uuid.UUID, note *string) (Version, error) {
	var value Version
	var raw []byte
	err := tx.QueryRow(ctx, `INSERT INTO message_template_versions(team_id,template_id,version_number,from_email,from_name,reply_to_email,subject,html_body,text_body,variables,based_on_version_id,change_note) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id,team_id,template_id,version_number,from_email,from_name,reply_to_email,subject,html_body,text_body,variables,based_on_version_id,change_note,created_at`, teamID, templateID, number, fromEmail, fromName, replyTo, subject, htmlBody, textBody, variables, basedOn, note).Scan(&value.ID, &value.TeamID, &value.TemplateID, &value.VersionNumber, &value.FromEmail, &value.FromName, &value.ReplyToEmail, &value.Subject, &value.HTML, &value.Text, &raw, &value.BasedOnVersionID, &value.ChangeNote, &value.CreatedAt)
	if err != nil {
		return Version{}, err
	}
	if err = json.Unmarshal(raw, &value.Variables); err != nil {
		return Version{}, err
	}
	return value, nil
}

func mapWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "alias") {
		return ErrAliasConflict
	}
	return fmt.Errorf("write message template: %w", err)
}
