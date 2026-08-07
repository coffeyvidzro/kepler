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

	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
)

var (
	ErrNotFound        = errors.New("message template not found")
	ErrVersionNotFound = errors.New("message template version not found")
	ErrAliasConflict   = errors.New("message template alias already exists")
	ErrVersionConflict = errors.New("message template version conflict")
)

type Repository struct {
	db      *pgxpool.Pool
	queries *dbsqlc.Queries
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db, queries: dbsqlc.New(db)}
}

func (r *Repository) Create(ctx context.Context, teamID uuid.UUID, req CreateRequest) (Template, Version, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Template{}, Version{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := r.queries.WithTx(tx)

	row, err := queries.CreateMessageTemplate(ctx, dbsqlc.CreateMessageTemplateParams{
		TeamID: teamID,
		Name:   req.Name,
		Alias:  req.Alias,
	})
	if err != nil {
		return Template{}, Version{}, mapWriteError(err)
	}

	variables, err := encodeVariables(req.Variables)
	if err != nil {
		return Template{}, Version{}, err
	}
	versionRow, err := queries.CreateMessageTemplateVersion(ctx, dbsqlc.CreateMessageTemplateVersionParams{
		TeamID:           teamID,
		TemplateID:       row.ID,
		VersionNumber:    row.NextVersionNumber,
		FromEmail:        req.FromEmail,
		FromName:         req.FromName,
		ReplyToEmail:     req.ReplyTo,
		Subject:          req.Subject,
		HtmlBody:         req.HTML,
		TextBody:         req.Text,
		Variables:        variables,
		BasedOnVersionID: nil,
		ChangeNote:       nil,
	})
	if err != nil {
		return Template{}, Version{}, err
	}
	version, err := versionFromSQLC(versionRow)
	if err != nil {
		return Template{}, Version{}, err
	}

	row, err = queries.SetMessageTemplateCurrentVersion(ctx, dbsqlc.SetMessageTemplateCurrentVersionParams{
		VersionID: &versionRow.ID,
		ID:        row.ID,
		TeamID:    teamID,
	})
	if err != nil {
		return Template{}, Version{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Template{}, Version{}, err
	}
	return templateFromSQLC(row), version, nil
}

func (r *Repository) List(ctx context.Context, teamID uuid.UUID, limit, offset int32) ([]Template, error) {
	rows, err := r.queries.ListMessageTemplates(ctx, dbsqlc.ListMessageTemplatesParams{
		TeamID: teamID, PageOffset: offset, PageLimit: limit,
	})
	if err != nil {
		return nil, err
	}
	return templatesFromSQLC(rows), nil
}

func (r *Repository) Resolve(ctx context.Context, teamID uuid.UUID, identifier string) (Template, error) {
	var (
		row dbsqlc.MessageTemplate
		err error
	)
	if id, parseErr := uuid.Parse(identifier); parseErr == nil {
		row, err = r.queries.GetMessageTemplateByID(ctx, dbsqlc.GetMessageTemplateByIDParams{ID: id, TeamID: teamID})
	} else {
		row, err = r.queries.GetMessageTemplateByAlias(ctx, dbsqlc.GetMessageTemplateByAliasParams{TeamID: teamID, Alias: identifier})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return Template{}, ErrNotFound
	}
	if err != nil {
		return Template{}, err
	}
	return templateFromSQLC(row), nil
}

func (r *Repository) GetVersion(ctx context.Context, teamID, templateID, versionID uuid.UUID) (Version, error) {
	row, err := r.queries.GetMessageTemplateVersion(ctx, dbsqlc.GetMessageTemplateVersionParams{
		ID: versionID, TemplateID: templateID, TeamID: teamID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, ErrVersionNotFound
	}
	if err != nil {
		return Version{}, err
	}
	return versionFromSQLC(row)
}

func (r *Repository) GetVersionTx(ctx context.Context, tx pgx.Tx, teamID, templateID, versionID uuid.UUID) (Version, error) {
	row, err := r.queries.WithTx(tx).GetMessageTemplateVersion(ctx, dbsqlc.GetMessageTemplateVersionParams{
		ID: versionID, TemplateID: templateID, TeamID: teamID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, ErrVersionNotFound
	}
	if err != nil {
		return Version{}, err
	}
	return versionFromSQLC(row)
}

func (r *Repository) ListVersions(ctx context.Context, teamID, templateID uuid.UUID, limit, offset int32) ([]Version, error) {
	rows, err := r.queries.ListMessageTemplateVersions(ctx, dbsqlc.ListMessageTemplateVersionsParams{
		TeamID: teamID, TemplateID: templateID, PageOffset: offset, PageLimit: limit,
	})
	if err != nil {
		return nil, err
	}
	result := make([]Version, 0, len(rows))
	for _, row := range rows {
		value, convertErr := versionFromSQLC(row)
		if convertErr != nil {
			return nil, convertErr
		}
		result = append(result, value)
	}
	return result, nil
}

func (r *Repository) Update(ctx context.Context, teamID uuid.UUID, template Template, base Version, req UpdateRequest) (Template, Version, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Template{}, Version{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := r.queries.WithTx(tx)
	templateID := uuid.MustParse(template.ID)

	locked, err := queries.LockMessageTemplate(ctx, dbsqlc.LockMessageTemplateParams{ID: templateID, TeamID: teamID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Template{}, Version{}, ErrNotFound
	}
	if err != nil {
		return Template{}, Version{}, err
	}
	if locked.CurrentVersionID == nil || locked.CurrentVersionID.String() != base.ID {
		return Template{}, Version{}, ErrVersionConflict
	}

	name, alias := template.Name, template.Alias
	if req.Name != nil {
		name = *req.Name
	}
	if req.Alias != nil {
		alias = *req.Alias
	}
	if _, err = queries.UpdateMessageTemplateMetadata(ctx, dbsqlc.UpdateMessageTemplateMetadataParams{
		Name: name, Alias: alias, ID: templateID, TeamID: teamID,
	}); err != nil {
		return Template{}, Version{}, mapWriteError(err)
	}

	fromEmail, fromName, replyTo := base.FromEmail, base.FromName, base.ReplyToEmail
	subject, htmlBody, textBody, variables := base.Subject, base.HTML, base.Text, base.Variables
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
	basedOn := uuid.MustParse(base.ID)
	versionRow, err := queries.CreateMessageTemplateVersion(ctx, dbsqlc.CreateMessageTemplateVersionParams{
		TeamID: teamID, TemplateID: templateID, VersionNumber: locked.NextVersionNumber,
		FromEmail: fromEmail, FromName: fromName, ReplyToEmail: replyTo,
		Subject: subject, HtmlBody: htmlBody, TextBody: textBody, Variables: encoded,
		BasedOnVersionID: &basedOn, ChangeNote: req.ChangeNote,
	})
	if err != nil {
		return Template{}, Version{}, err
	}
	version, err := versionFromSQLC(versionRow)
	if err != nil {
		return Template{}, Version{}, err
	}
	finalRow, err := queries.SetMessageTemplateCurrentVersion(ctx, dbsqlc.SetMessageTemplateCurrentVersionParams{
		VersionID: &versionRow.ID, ID: templateID, TeamID: teamID,
	})
	if err != nil {
		return Template{}, Version{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Template{}, Version{}, err
	}
	return templateFromSQLC(finalRow), version, nil
}

func (r *Repository) Publish(ctx context.Context, teamID uuid.UUID, templateID, versionID uuid.UUID) (Template, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Template{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := r.queries.WithTx(tx)

	exists, err := queries.MessageTemplateVersionExists(ctx, dbsqlc.MessageTemplateVersionExistsParams{
		VersionID: versionID, TemplateID: templateID, TeamID: teamID,
	})
	if err != nil {
		return Template{}, err
	}
	if !exists {
		return Template{}, ErrVersionNotFound
	}
	row, err := queries.PublishMessageTemplateVersion(ctx, dbsqlc.PublishMessageTemplateVersionParams{
		VersionID: &versionID, ID: templateID, TeamID: teamID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Template{}, ErrNotFound
	}
	if err != nil {
		return Template{}, err
	}
	if _, err = queries.CreateMessageTemplatePublication(ctx, dbsqlc.CreateMessageTemplatePublicationParams{
		TeamID: teamID, TemplateID: templateID, VersionID: versionID,
	}); err != nil {
		return Template{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Template{}, err
	}
	return templateFromSQLC(row), nil
}

func (r *Repository) Delete(ctx context.Context, teamID, templateID uuid.UUID) (Template, error) {
	row, err := r.queries.SoftDeleteMessageTemplate(ctx, dbsqlc.SoftDeleteMessageTemplateParams{ID: templateID, TeamID: teamID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Template{}, ErrNotFound
	}
	if err != nil {
		return Template{}, err
	}
	return templateFromSQLC(row), nil
}

func (r *Repository) CursorExists(ctx context.Context, teamID, cursorID uuid.UUID) (bool, error) {
	return r.queries.MessageTemplateCursorExists(ctx, dbsqlc.MessageTemplateCursorExistsParams{
		CursorID: cursorID, TeamID: teamID,
	})
}

func (r *Repository) ListPage(ctx context.Context, teamID uuid.UUID, limit int32, after, before *uuid.UUID) ([]Template, error) {
	var (
		rows []dbsqlc.MessageTemplate
		err  error
	)
	switch {
	case after != nil:
		rows, err = r.queries.ListMessageTemplatesAfter(ctx, dbsqlc.ListMessageTemplatesAfterParams{
			ScopeTeamID: teamID, CursorID: *after, PageLimit: limit,
		})
	case before != nil:
		rows, err = r.queries.ListMessageTemplatesBefore(ctx, dbsqlc.ListMessageTemplatesBeforeParams{
			ScopeTeamID: teamID, CursorID: *before, PageLimit: limit,
		})
	default:
		rows, err = r.queries.ListMessageTemplates(ctx, dbsqlc.ListMessageTemplatesParams{
			TeamID: teamID, PageOffset: 0, PageLimit: limit,
		})
	}
	if err != nil {
		return nil, err
	}
	return templatesFromSQLC(rows), nil
}

func templateFromSQLC(row dbsqlc.MessageTemplate) Template {
	value := Template{
		ID: row.ID.String(), TeamID: row.TeamID.String(), Name: row.Name, Alias: row.Alias,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if row.CurrentVersionID != nil {
		id := row.CurrentVersionID.String()
		value.CurrentVersionID = &id
	}
	if row.PublishedVersionID != nil {
		id := row.PublishedVersionID.String()
		value.PublishedVersionID = &id
	}
	if row.PublishedAt.Valid {
		publishedAt := row.PublishedAt.Time
		value.PublishedAt = &publishedAt
	}
	value.HasUnpublishedChanges = value.CurrentVersionID != nil && (value.PublishedVersionID == nil || *value.CurrentVersionID != *value.PublishedVersionID)
	return value
}

func templatesFromSQLC(rows []dbsqlc.MessageTemplate) []Template {
	values := make([]Template, 0, len(rows))
	for _, row := range rows {
		values = append(values, templateFromSQLC(row))
	}
	return values
}

func versionFromSQLC(row dbsqlc.MessageTemplateVersion) (Version, error) {
	variables := make([]Variable, 0)
	if err := json.Unmarshal(row.Variables, &variables); err != nil {
		return Version{}, err
	}
	value := Version{
		ID: row.ID.String(), TeamID: row.TeamID.String(), TemplateID: row.TemplateID.String(),
		VersionNumber: row.VersionNumber, FromEmail: row.FromEmail, FromName: row.FromName,
		ReplyToEmail: row.ReplyToEmail, Subject: row.Subject, HTML: row.HtmlBody, Text: row.TextBody,
		Variables: variables, ChangeNote: row.ChangeNote, CreatedAt: row.CreatedAt.Time,
	}
	if row.BasedOnVersionID != nil {
		id := row.BasedOnVersionID.String()
		value.BasedOnVersionID = &id
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
