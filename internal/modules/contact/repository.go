package contact

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
)

var (
	ErrAlreadyExists        = errors.New("contact already exists")
	ErrUnknownProperty      = errors.New("unknown contact property")
	ErrPropertyTypeMismatch = errors.New("contact property type mismatch")
	ErrContactNotFound      = errors.New("contact not found")
	ErrSegmentNotFound      = errors.New("segment not found")
)

type Repository struct {
	db      *pgxpool.Pool
	emitter eventEmitter
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func NewRepositoryWithEventEmitter(db *pgxpool.Pool, emitter eventEmitter) *Repository {
	return &Repository{db: db, emitter: emitter}
}

func (r *Repository) Create(ctx context.Context, teamID uuid.UUID, req CreateRequest) (Contact, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Contact{}, fmt.Errorf("begin contact creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var result Contact
	err = tx.QueryRow(ctx, `
		INSERT INTO contacts (team_id, email, first_name, last_name, unsubscribed)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, team_id, email, first_name, last_name, unsubscribed, created_at, updated_at
	`, teamID, req.Email, req.FirstName, req.LastName, req.Unsubscribed).Scan(
		&result.ID,
		&result.TeamID,
		&result.Email,
		&result.FirstName,
		&result.LastName,
		&result.Unsubscribed,
		&result.CreatedAt,
		&result.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Contact{}, ErrAlreadyExists
		}
		return Contact{}, fmt.Errorf("create contact: %w", err)
	}

	contactID, err := uuid.Parse(result.ID)
	if err != nil {
		return Contact{}, fmt.Errorf("parse created contact id: %w", err)
	}
	if err := replaceProperties(ctx, tx, teamID, contactID, req.Properties); err != nil {
		return Contact{}, err
	}
	result.Properties = cloneProperties(req.Properties)
	if err := emitContactEvent(ctx, tx, r.emitter, platformevent.TypeContactCreated, result, nil); err != nil {
		return Contact{}, fmt.Errorf("emit contact created event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Contact{}, fmt.Errorf("commit contact creation: %w", err)
	}
	return result, nil
}

func (r *Repository) List(ctx context.Context, teamID uuid.UUID, limit, offset int32) ([]Contact, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, team_id, email, first_name, last_name, unsubscribed, created_at, updated_at
		FROM contacts
		WHERE team_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`, teamID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list contacts: %w", err)
	}
	defer rows.Close()

	contacts := make([]Contact, 0)
	for rows.Next() {
		var value Contact
		if err := rows.Scan(&value.ID, &value.TeamID, &value.Email, &value.FirstName, &value.LastName, &value.Unsubscribed, &value.CreatedAt, &value.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan contact: %w", err)
		}
		contacts = append(contacts, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate contacts: %w", err)
	}

	for i := range contacts {
		contactID, parseErr := uuid.Parse(contacts[i].ID)
		if parseErr != nil {
			return nil, fmt.Errorf("parse contact id: %w", parseErr)
		}
		properties, loadErr := loadProperties(ctx, r.db, teamID, contactID)
		if loadErr != nil {
			return nil, loadErr
		}
		contacts[i].Properties = properties
	}
	return contacts, nil
}

func (r *Repository) Get(ctx context.Context, id, teamID uuid.UUID) (Contact, error) {
	return getContact(ctx, r.db, id, teamID)
}

func (r *Repository) Update(ctx context.Context, id, teamID uuid.UUID, req CreateRequest) (Contact, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Contact{}, fmt.Errorf("begin contact update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	previous, err := getContact(ctx, tx, id, teamID)
	if err != nil {
		return Contact{}, err
	}

	var result Contact
	err = tx.QueryRow(ctx, `
		UPDATE contacts
		SET email = $3,
			first_name = $4,
			last_name = $5,
			unsubscribed = $6,
			updated_at = now()
		WHERE id = $1 AND team_id = $2
		RETURNING id, team_id, email, first_name, last_name, unsubscribed, created_at, updated_at
	`, id, teamID, req.Email, req.FirstName, req.LastName, req.Unsubscribed).Scan(
		&result.ID,
		&result.TeamID,
		&result.Email,
		&result.FirstName,
		&result.LastName,
		&result.Unsubscribed,
		&result.CreatedAt,
		&result.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Contact{}, ErrAlreadyExists
		}
		return Contact{}, err
	}
	if err := replaceProperties(ctx, tx, teamID, id, req.Properties); err != nil {
		return Contact{}, err
	}
	result.Properties = cloneProperties(req.Properties)
	if err := emitContactEvent(ctx, tx, r.emitter, platformevent.TypeContactUpdated, result, &previous); err != nil {
		return Contact{}, fmt.Errorf("emit contact updated event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Contact{}, fmt.Errorf("commit contact update: %w", err)
	}
	return result, nil
}

func (r *Repository) Delete(ctx context.Context, id, teamID uuid.UUID) (Contact, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Contact{}, fmt.Errorf("begin contact deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	result, err := getContact(ctx, tx, id, teamID)
	if err != nil {
		return Contact{}, err
	}
	command, err := tx.Exec(ctx, `DELETE FROM contacts WHERE id = $1 AND team_id = $2`, id, teamID)
	if err != nil {
		return Contact{}, err
	}
	if command.RowsAffected() == 0 {
		return Contact{}, pgx.ErrNoRows
	}
	if err := emitContactEvent(ctx, tx, r.emitter, platformevent.TypeContactDeleted, result, nil); err != nil {
		return Contact{}, fmt.Errorf("emit contact deleted event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Contact{}, fmt.Errorf("commit contact deletion: %w", err)
	}
	return result, nil
}

func (r *Repository) ListSegments(ctx context.Context, contactID, teamID uuid.UUID) ([]SegmentMembership, error) {
	if err := ensureContactExists(ctx, r.db, contactID, teamID, false); err != nil {
		return nil, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT segment.id, segment.team_id, segment.name, segment.created_at, membership.created_at
		FROM contact_segments AS membership
		JOIN segments AS segment
		  ON segment.id = membership.segment_id
		 AND segment.team_id = membership.team_id
		WHERE membership.team_id = $1
		  AND membership.contact_id = $2
		ORDER BY membership.created_at DESC, segment.id DESC
	`, teamID, contactID)
	if err != nil {
		return nil, fmt.Errorf("list contact segments: %w", err)
	}
	defer rows.Close()

	memberships := make([]SegmentMembership, 0)
	for rows.Next() {
		var membership SegmentMembership
		if err := rows.Scan(&membership.ID, &membership.TeamID, &membership.Name, &membership.CreatedAt, &membership.AssignedAt); err != nil {
			return nil, fmt.Errorf("scan contact segment membership: %w", err)
		}
		memberships = append(memberships, membership)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate contact segment memberships: %w", err)
	}
	return memberships, nil
}

func (r *Repository) AddSegment(ctx context.Context, contactID, segmentID, teamID uuid.UUID) (SegmentMembership, bool, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return SegmentMembership{}, false, fmt.Errorf("begin contact segment assignment: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := ensureContactExists(ctx, tx, contactID, teamID, true); err != nil {
		return SegmentMembership{}, false, err
	}
	membership, err := getMembershipSegment(ctx, tx, segmentID, teamID, true)
	if err != nil {
		return SegmentMembership{}, false, err
	}

	created := true
	err = tx.QueryRow(ctx, `
		INSERT INTO contact_segments (team_id, contact_id, segment_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (contact_id, segment_id) DO NOTHING
		RETURNING created_at
	`, teamID, contactID, segmentID).Scan(&membership.AssignedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		created = false
		err = tx.QueryRow(ctx, `
			SELECT created_at FROM contact_segments
			WHERE team_id = $1 AND contact_id = $2 AND segment_id = $3
		`, teamID, contactID, segmentID).Scan(&membership.AssignedAt)
	}
	if err != nil {
		return SegmentMembership{}, false, fmt.Errorf("assign contact to segment: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return SegmentMembership{}, false, fmt.Errorf("commit contact segment assignment: %w", err)
	}
	return membership, created, nil
}

func (r *Repository) RemoveSegment(ctx context.Context, contactID, segmentID, teamID uuid.UUID) (bool, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, fmt.Errorf("begin contact segment removal: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := ensureContactExists(ctx, tx, contactID, teamID, true); err != nil {
		return false, err
	}
	if _, err := getMembershipSegment(ctx, tx, segmentID, teamID, true); err != nil {
		return false, err
	}
	command, err := tx.Exec(ctx, `
		DELETE FROM contact_segments
		WHERE team_id = $1 AND contact_id = $2 AND segment_id = $3
	`, teamID, contactID, segmentID)
	if err != nil {
		return false, fmt.Errorf("remove contact from segment: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit contact segment removal: %w", err)
	}
	return command.RowsAffected() > 0, nil
}

func ensureContactExists(ctx context.Context, db contactQueryer, contactID, teamID uuid.UUID, lock bool) error {
	query := `SELECT 1 FROM contacts WHERE id = $1 AND team_id = $2`
	if lock {
		query += ` FOR SHARE`
	}
	var exists int
	err := db.QueryRow(ctx, query, contactID, teamID).Scan(&exists)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrContactNotFound
	}
	if err != nil {
		return fmt.Errorf("validate contact for segment membership: %w", err)
	}
	return nil
}

func getMembershipSegment(ctx context.Context, db contactQueryer, segmentID, teamID uuid.UUID, lock bool) (SegmentMembership, error) {
	query := `SELECT id, team_id, name, created_at FROM segments WHERE id = $1 AND team_id = $2`
	if lock {
		query += ` FOR SHARE`
	}
	var membership SegmentMembership
	err := db.QueryRow(ctx, query, segmentID, teamID).Scan(&membership.ID, &membership.TeamID, &membership.Name, &membership.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return SegmentMembership{}, ErrSegmentNotFound
	}
	if err != nil {
		return SegmentMembership{}, fmt.Errorf("validate segment for contact membership: %w", err)
	}
	return membership, nil
}

type contactQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func getContact(ctx context.Context, db contactQueryer, id, teamID uuid.UUID) (Contact, error) {
	var result Contact
	err := db.QueryRow(ctx, `
		SELECT id, team_id, email, first_name, last_name, unsubscribed, created_at, updated_at
		FROM contacts
		WHERE id = $1 AND team_id = $2
	`, id, teamID).Scan(
		&result.ID,
		&result.TeamID,
		&result.Email,
		&result.FirstName,
		&result.LastName,
		&result.Unsubscribed,
		&result.CreatedAt,
		&result.UpdatedAt,
	)
	if err != nil {
		return Contact{}, err
	}
	result.Properties, err = loadProperties(ctx, db, teamID, id)
	if err != nil {
		return Contact{}, err
	}
	return result, nil
}

func replaceProperties(ctx context.Context, tx pgx.Tx, teamID, contactID uuid.UUID, properties map[string]any) error {
	if _, err := tx.Exec(ctx, `DELETE FROM contact_property_values WHERE team_id = $1 AND contact_id = $2`, teamID, contactID); err != nil {
		return fmt.Errorf("clear contact properties: %w", err)
	}
	for key, value := range properties {
		var propertyID uuid.UUID
		var valueType string
		err := tx.QueryRow(ctx, `
			SELECT id, value_type
			FROM contact_properties
			WHERE team_id = $1 AND key = $2
		`, teamID, key).Scan(&propertyID, &valueType)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: %s", ErrUnknownProperty, key)
		}
		if err != nil {
			return fmt.Errorf("get contact property %q: %w", key, err)
		}

		switch valueType {
		case "string":
			stringValue, ok := value.(string)
			if !ok {
				return fmt.Errorf("%w: %s must be a string", ErrPropertyTypeMismatch, key)
			}
			_, err = tx.Exec(ctx, `
				INSERT INTO contact_property_values (
					team_id, contact_id, contact_property_id, value_type, string_value
				) VALUES ($1, $2, $3, 'string', $4)
			`, teamID, contactID, propertyID, stringValue)
		case "number":
			numberValue, ok := numericValue(value)
			if !ok {
				return fmt.Errorf("%w: %s must be a number", ErrPropertyTypeMismatch, key)
			}
			_, err = tx.Exec(ctx, `
				INSERT INTO contact_property_values (
					team_id, contact_id, contact_property_id, value_type, number_value
				) VALUES ($1, $2, $3, 'number', $4)
			`, teamID, contactID, propertyID, numberValue)
		default:
			return fmt.Errorf("unsupported contact property type %q", valueType)
		}
		if err != nil {
			return fmt.Errorf("store contact property %q: %w", key, err)
		}
	}
	return nil
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func loadProperties(ctx context.Context, db queryer, teamID, contactID uuid.UUID) (map[string]any, error) {
	rows, err := db.Query(ctx, `
		SELECT cp.key, cpv.value_type, cpv.string_value, cpv.number_value::text
		FROM contact_property_values AS cpv
		JOIN contact_properties AS cp
		  ON cp.id = cpv.contact_property_id
		 AND cp.team_id = cpv.team_id
		WHERE cpv.team_id = $1 AND cpv.contact_id = $2
		ORDER BY cp.key
	`, teamID, contactID)
	if err != nil {
		return nil, fmt.Errorf("load contact properties: %w", err)
	}
	defer rows.Close()

	properties := make(map[string]any)
	for rows.Next() {
		var key, valueType string
		var stringValue, numberText *string
		if err := rows.Scan(&key, &valueType, &stringValue, &numberText); err != nil {
			return nil, fmt.Errorf("scan contact property: %w", err)
		}
		if valueType == "string" && stringValue != nil {
			properties[key] = *stringValue
		}
		if valueType == "number" && numberText != nil {
			numberValue, parseErr := strconv.ParseFloat(*numberText, 64)
			if parseErr != nil {
				return nil, fmt.Errorf("parse contact property %q: %w", key, parseErr)
			}
			properties[key] = numberValue
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate contact properties: %w", err)
	}
	return properties, nil
}

func numericValue(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int32:
		return float64(number), true
	case int64:
		return float64(number), true
	case uint:
		return float64(number), true
	case uint32:
		return float64(number), true
	case uint64:
		return float64(number), true
	default:
		return 0, false
	}
}

func cloneProperties(input map[string]any) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && strings.EqualFold(pgErr.Code, "23505")
}

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
			ScopeContactID: contactID,
			ScopeTeamID:    teamID,
			CursorID:       cursorID,
			PageLimit:      limit,
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
			ScopeContactID: contactID,
			ScopeTeamID:    teamID,
			CursorID:       cursorID,
			PageLimit:      limit,
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
			ScopeContactID: contactID,
			ScopeTeamID:    teamID,
			PageLimit:      limit,
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
			TeamID:       teamID,
			ContactID:    contactID,
			TopicID:      topicID,
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
