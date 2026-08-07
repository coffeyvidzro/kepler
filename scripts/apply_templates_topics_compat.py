from pathlib import Path


def replace(path: str, old: str, new: str) -> None:
    file = Path(path)
    text = file.read_text()
    if old not in text:
        raise SystemExit(f"missing replacement in {path}: {old[:80]!r}")
    file.write_text(text.replace(old, new, 1))


replace(
    "internal/modules/messagetemplate/model.go",
    '\tAlias                 string     `json:"alias"`',
    '\tAlias                 *string    `json:"alias"`',
)
replace(
    "internal/modules/messagetemplate/model.go",
    '\tAlias     string     `json:"alias"`',
    '\tAlias     *string    `json:"alias,omitempty"`',
)
replace(
    "internal/modules/messagetemplate/model.go",
    '\tAlias         *string     `json:"alias,omitempty"`',
    '\tAlias         **string    `json:"alias,omitempty"`',
)
replace(
    "internal/modules/messagetemplate/model.go",
    'type DuplicateRequest struct {\n\tName  string `json:"name"`\n\tAlias string `json:"alias"`\n}',
    'type DuplicateRequest struct {\n\tName  string  `json:"name,omitempty"`\n\tAlias *string `json:"alias,omitempty"`\n}',
)
replace(
    "migrations/029_create_message_templates.sql",
    "    alias VARCHAR(100) NOT NULL,",
    "    alias VARCHAR(100),",
)
replace(
    "migrations/029_create_message_templates.sql",
    "    CONSTRAINT chk_message_template_versions_subject CHECK (length(btrim(subject)) > 0),\n",
    "",
)

service = Path("internal/modules/messagetemplate/service.go")
text = service.read_text()
start = text.index("func validateCreate(req CreateRequest)")
end = text.index("func requireAccess(", start)
validation = '''func validateCreate(req CreateRequest) (CreateRequest, error) {
\treq.Name = strings.TrimSpace(req.Name)
\treq.Alias = normalizeOptional(req.Alias)
\treq.Subject = strings.TrimSpace(req.Subject)
\tif req.Name == "" || utf8.RuneCountInString(req.Name) > maxTemplateNameCharacters {
\t\treturn req, apperrors.NewBadRequest("Template name is required and must be at most 100 characters")
\t}
\tif err := validateAlias(req.Alias); err != nil {
\t\treturn req, err
\t}
\tif err := validateContent(req.Subject, req.HTML, req.FromEmail, req.ReplyTo, req.Variables); err != nil {
\t\treturn req, err
\t}
\treq.FromEmail = normalizeOptional(req.FromEmail)
\treq.FromName = normalizeOptional(req.FromName)
\treturn req, nil
}

func validateUpdate(template Template, base Version, req *UpdateRequest) error {
\tname, alias := template.Name, template.Alias
\tif req.Name != nil {
\t\tname = strings.TrimSpace(*req.Name)
\t\treq.Name = &name
\t}
\tif req.Alias != nil {
\t\talias = normalizeOptional(*req.Alias)
\t\treq.Alias = &alias
\t}
\tif name == "" || utf8.RuneCountInString(name) > maxTemplateNameCharacters {
\t\treturn apperrors.NewBadRequest("Template name is required and must be at most 100 characters")
\t}
\tif err := validateAlias(alias); err != nil {
\t\treturn err
\t}
\tsubject, htmlBody, variables := base.Subject, base.HTML, base.Variables
\tfromEmail, replyTo := base.FromEmail, base.ReplyToEmail
\tif req.Subject != nil {
\t\tsubject = strings.TrimSpace(*req.Subject)
\t\treq.Subject = &subject
\t}
\tif req.HTML != nil {
\t\thtmlBody = *req.HTML
\t}
\tif req.Variables != nil {
\t\tvariables = *req.Variables
\t}
\tif req.FromEmail != nil {
\t\tfromEmail = *req.FromEmail
\t}
\tif req.ReplyTo != nil {
\t\treplyTo = *req.ReplyTo
\t}
\treturn validateContent(subject, htmlBody, fromEmail, replyTo, variables)
}

func validateVersion(version Version) error {
\tif strings.TrimSpace(version.Subject) == "" {
\t\treturn apperrors.NewBadRequest("Template subject is required before publishing")
\t}
\treturn validateContent(version.Subject, version.HTML, version.FromEmail, version.ReplyToEmail, version.Variables)
}

func validateAlias(alias *string) error {
\tif alias == nil {
\t\treturn nil
\t}
\tif len(*alias) > maxTemplateAliasCharacters || !aliasPattern.MatchString(*alias) {
\t\treturn apperrors.NewBadRequest("Template alias must use letters, numbers, underscores, or dashes and be at most 100 characters")
\t}
\treturn nil
}

func validateContent(subject, htmlBody string, fromEmail, replyTo *string, variables []Variable) error {
\tif utf8.RuneCountInString(subject) > maxTemplateSubjectChars {
\t\treturn apperrors.NewBadRequest("Template subject must be at most 255 characters")
\t}
\tif strings.TrimSpace(htmlBody) == "" {
\t\treturn apperrors.NewBadRequest("Template HTML is required")
\t}
\tif len(variables) > maxTemplateVariables {
\t\treturn apperrors.NewBadRequest("Template may define at most 50 variables")
\t}
\tdefinitions := map[string]struct{}{}
\tfor _, variable := range variables {
\t\tif !variableKeyPattern.MatchString(variable.Key) {
\t\t\treturn apperrors.NewBadRequest("Template variable keys must start with a letter and use only letters, numbers, and underscores")
\t\t}
\t\tupper := strings.ToUpper(variable.Key)
\t\tif _, reserved := reservedVariables[upper]; reserved {
\t\t\treturn apperrors.NewBadRequest("Template variable key is reserved: " + variable.Key)
\t\t}
\t\tif _, exists := definitions[variable.Key]; exists {
\t\t\treturn apperrors.NewBadRequest("Template variable keys must be unique")
\t\t}
\t\tdefinitions[variable.Key] = struct{}{}
\t\tif variable.Type != VariableTypeString && variable.Type != VariableTypeNumber {
\t\t\treturn apperrors.NewBadRequest("Template variable type must be string or number")
\t\t}
\t\tif variable.FallbackValue != nil {
\t\t\tif _, err := renderVariableValue(variable, variable.FallbackValue); err != nil {
\t\t\t\treturn apperrors.NewBadRequest(err.Error())
\t\t\t}
\t\t}
\t}
\tfor _, key := range referencedVariables(subject, htmlBody) {
\t\tif _, exists := definitions[key]; !exists {
\t\t\treturn apperrors.NewBadRequest("Unknown template variable: " + key)
\t\t}
\t}
\tif err := validateStoredEmail(fromEmail, "Template sender"); err != nil {
\t\treturn err
\t}
\tif replyTo != nil {
\t\tvalues, err := decodeStoredReplyTo(replyTo)
\t\tif err != nil {
\t\t\treturn apperrors.NewBadRequest("Template reply-to addresses are invalid")
\t\t}
\t\tfor _, value := range values {
\t\t\taddress, parseErr := mail.ParseAddress(strings.TrimSpace(value))
\t\t\tif parseErr != nil || address.Address == "" {
\t\t\t\treturn apperrors.NewBadRequest("Template reply-to addresses are invalid")
\t\t\t}
\t\t}
\t}
\treturn nil
}

func validateStoredEmail(value *string, label string) error {
\tif value == nil {
\t\treturn nil
\t}
\tparsed, err := mail.ParseAddress(strings.TrimSpace(*value))
\tif err != nil || !strings.EqualFold(parsed.Address, strings.TrimSpace(*value)) {
\t\treturn apperrors.NewBadRequest(label + " must be a valid email address")
\t}
\treturn nil
}

func normalizeOptional(value *string) *string {
\tif value == nil {
\t\treturn nil
\t}
\ttrimmed := strings.TrimSpace(*value)
\tif trimmed == "" {
\t\treturn nil
\t}
\treturn &trimmed
}

func normalizeList(req *ListRequest) {
\tif req.Limit <= 0 || req.Limit > 100 {
\t\treq.Limit = 50
\t}
\tif req.Offset < 0 {
\t\treq.Offset = 0
\t}
}

'''
text = text[:start] + validation + text[end:]
text = text.replace(
    '"FIRST_NAME": {}, "LAST_NAME": {}, "EMAIL": {}, "UNSUBSCRIBE_URL": {},',
    '"FIRST_NAME": {}, "LAST_NAME": {}, "EMAIL": {}, "UNSUBSCRIBE_URL": {}, "RESEND_UNSUBSCRIBE_URL": {},',
)
old_duplicate = '\tcreate := CreateRequest{Name: req.Name, Alias: req.Alias, FromEmail: version.FromEmail, FromName: version.FromName, ReplyTo: version.ReplyToEmail, Subject: version.Subject, HTML: version.HTML, Text: version.Text, Variables: version.Variables}\n'
new_duplicate = '\tname := strings.TrimSpace(req.Name)\n\tif name == "" {\n\t\tname = source.Name + " Copy"\n\t}\n\tcreate := CreateRequest{Name: name, Alias: req.Alias, FromEmail: version.FromEmail, FromName: version.FromName, ReplyTo: version.ReplyToEmail, Subject: version.Subject, HTML: version.HTML, Text: version.Text, Variables: version.Variables}\n'
if old_duplicate not in text:
    raise SystemExit("duplicate mapping not found")
service.write_text(text.replace(old_duplicate, new_duplicate, 1))

replace(
    "internal/modules/messagetemplate/compatibility.go",
    "\tmapped.Alias = request.Alias\n",
    "\tif request.Alias != nil {\n\t\tmapped.Alias = &request.Alias\n\t}\n",
)

template_repository = Path("internal/modules/messagetemplate/repository.go")
template_repository.write_text(template_repository.read_text().rstrip() + '''

func (r *Repository) CursorExists(ctx context.Context, teamID, cursorID uuid.UUID) (bool, error) {
\tvar exists bool
\terr := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM message_templates WHERE id=$1 AND team_id=$2 AND deleted_at IS NULL)`, cursorID, teamID).Scan(&exists)
\treturn exists, err
}

func (r *Repository) ListPage(ctx context.Context, teamID uuid.UUID, limit int32, after, before *uuid.UUID) ([]Template, error) {
\tquery := `SELECT id,team_id,name,alias,current_version_id,published_version_id,published_at,created_at,updated_at FROM message_templates WHERE team_id=$1 AND deleted_at IS NULL`
\targs := []any{teamID}
\tif after != nil {
\t\tquery += ` AND (created_at,id) < (SELECT created_at,id FROM message_templates WHERE id=$2 AND team_id=$1 AND deleted_at IS NULL)`
\t\targs = append(args, *after)
\t}
\tif before != nil {
\t\tquery += ` AND (created_at,id) > (SELECT created_at,id FROM message_templates WHERE id=$2 AND team_id=$1 AND deleted_at IS NULL)`
\t\targs = append(args, *before)
\t}
\tif before != nil {
\t\tquery += ` ORDER BY created_at ASC,id ASC`
\t} else {
\t\tquery += ` ORDER BY created_at DESC,id DESC`
\t}
\targs = append(args, limit)
\tquery += fmt.Sprintf(` LIMIT $%d`, len(args))
\trows, err := r.db.Query(ctx, query, args...)
\tif err != nil {
\t\treturn nil, err
\t}
\tdefer rows.Close()
\tresult := make([]Template, 0)
\tfor rows.Next() {
\t\tvar value Template
\t\tif err := rows.Scan(&value.ID, &value.TeamID, &value.Name, &value.Alias, &value.CurrentVersionID, &value.PublishedVersionID, &value.PublishedAt, &value.CreatedAt, &value.UpdatedAt); err != nil {
\t\t\treturn nil, err
\t\t}
\t\tvalue.HasUnpublishedChanges = value.CurrentVersionID != nil && (value.PublishedVersionID == nil || *value.CurrentVersionID != *value.PublishedVersionID)
\t\tresult = append(result, value)
\t}
\treturn result, rows.Err()
}
''')

replace(
    "internal/modules/topic/model.go",
    'type UpdateRequest struct {\n\tName        *string  `json:"name,omitempty"`\n\tDescription **string `json:"description,omitempty"`\n}',
    'type UpdateRequest struct {\n\tName        *string  `json:"name,omitempty"`\n\tDescription **string `json:"description,omitempty"`\n\tVisibility  *string  `json:"visibility,omitempty"`\n}',
)
replace(
    "internal/modules/topic/service.go",
    '\tif req.Visibility == "" {\n\t\treq.Visibility = "public"\n\t}',
    '\tif req.Visibility == "" {\n\t\treq.Visibility = "private"\n\t}',
)
replace(
    "internal/modules/topic/service.go",
    "\tname := current.Name\n\tdescription := current.Description",
    "\tname := current.Name\n\tdescription := current.Description\n\tvisibility := current.Visibility",
)
replace(
    "internal/modules/topic/service.go",
    "\tif req.Description != nil {\n\t\tdescription = normalizeOptional(*req.Description)\n\t}\n\tif err := validateNameDescription(name, description); err != nil {",
    "\tif req.Description != nil {\n\t\tdescription = normalizeOptional(*req.Description)\n\t}\n\tif req.Visibility != nil {\n\t\tvisibility = strings.ToLower(strings.TrimSpace(*req.Visibility))\n\t}\n\tif err := validateNameDescription(name, description); err != nil {",
)
replace(
    "internal/modules/topic/service.go",
    "\tresult, err := s.repository.Update(ctx, id, access.Scope.TeamID, name, description)",
    "\tif visibility != \"public\" && visibility != \"private\" {\n\t\treturn Topic{}, apperrors.NewBadRequest(\"Visibility must be public or private\")\n\t}\n\tresult, err := s.repository.Update(ctx, id, access.Scope.TeamID, name, description, visibility)",
)
replace(
    "internal/modules/topic/repository.go",
    "func (r *Repository) Update(ctx context.Context, id, teamID uuid.UUID, name string, description *string) (Topic, error) {",
    "func (r *Repository) Update(ctx context.Context, id, teamID uuid.UUID, name string, description *string, visibility string) (Topic, error) {",
)
replace(
    "internal/modules/topic/repository.go",
    "\t\tUPDATE topics SET name = $3, description = $4, updated_at = now()\n\t\tWHERE id = $1 AND team_id = $2",
    "\t\tUPDATE topics SET name = $3, description = $4, visibility = $5, updated_at = now()\n\t\tWHERE id = $1 AND team_id = $2",
)
replace(
    "internal/modules/topic/repository.go",
    "\t`, id, teamID, name, description).Scan",
    "\t`, id, teamID, name, description, visibility).Scan",
)

topic_repository = Path("internal/modules/topic/repository.go")
topic_repository.write_text(topic_repository.read_text().rstrip() + '''

func (r *Repository) CursorExists(ctx context.Context, teamID, cursorID uuid.UUID) (bool, error) {
\tvar exists bool
\terr := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM topics WHERE id=$1 AND team_id=$2)`, cursorID, teamID).Scan(&exists)
\treturn exists, err
}

func (r *Repository) ListPage(ctx context.Context, teamID uuid.UUID, limit int32, after, before *uuid.UUID) ([]Topic, error) {
\tquery := `SELECT id,team_id,name,description,default_subscription,visibility,created_at,updated_at FROM topics WHERE team_id=$1`
\targs := []any{teamID}
\tif after != nil {
\t\tquery += ` AND (created_at,id) < (SELECT created_at,id FROM topics WHERE id=$2 AND team_id=$1)`
\t\targs = append(args, *after)
\t}
\tif before != nil {
\t\tquery += ` AND (created_at,id) > (SELECT created_at,id FROM topics WHERE id=$2 AND team_id=$1)`
\t\targs = append(args, *before)
\t}
\tif before != nil {
\t\tquery += ` ORDER BY created_at ASC,id ASC`
\t} else {
\t\tquery += ` ORDER BY created_at DESC,id DESC`
\t}
\targs = append(args, limit)
\tquery += fmt.Sprintf(` LIMIT $%d`, len(args))
\trows, err := r.db.Query(ctx, query, args...)
\tif err != nil {
\t\treturn nil, fmt.Errorf("list topic page: %w", err)
\t}
\tdefer rows.Close()
\tvalues := make([]Topic, 0)
\tfor rows.Next() {
\t\tvar value Topic
\t\tif err := rows.Scan(&value.ID, &value.TeamID, &value.Name, &value.Description, &value.DefaultSubscription, &value.Visibility, &value.CreatedAt, &value.UpdatedAt); err != nil {
\t\t\treturn nil, fmt.Errorf("scan topic page: %w", err)
\t\t}
\t\tvalues = append(values, value)
\t}
\treturn values, rows.Err()
}
''')

Path("internal/modules/messagetemplate/compatibility_test.go").write_text('''package messagetemplate

import (
\t"encoding/json"
\t"testing"
)

func TestStringListAcceptsStringAndArray(t *testing.T) {
\tfor _, test := range []struct { input string; want int }{{`"reply@example.com"`, 1}, {`["a@example.com","b@example.com"]`, 2}} {
\t\tvar values StringList
\t\tif err := json.Unmarshal([]byte(test.input), &values); err != nil { t.Fatalf("unmarshal %s: %v", test.input, err) }
\t\tif len(values) != test.want { t.Fatalf("unmarshal %s length = %d, want %d", test.input, len(values), test.want) }
\t}
}

func TestTemplateMutationResponseContract(t *testing.T) {
\tdata, err := json.Marshal(MutationResponse{Object: ObjectTemplate, ID: "template-id"})
\tif err != nil { t.Fatal(err) }
\tif string(data) != `{"object":"template","id":"template-id"}` { t.Fatalf("unexpected response: %s", data) }
}

func TestNormalizeAPIListRequest(t *testing.T) {
\trequest := APIListRequest{}
\tif err := normalizeAPIListRequest(&request); err != nil { t.Fatal(err) }
\tif request.Limit != 20 { t.Fatalf("default limit = %d", request.Limit) }
\tif err := normalizeAPIListRequest(&APIListRequest{Limit: 101}); err == nil { t.Fatal("expected invalid limit") }
\tif err := normalizeAPIListRequest(&APIListRequest{After: uuidText, Before: uuidText}); err == nil { t.Fatal("expected mutually exclusive cursors") }
}

const uuidText = "b6d24b8e-af0b-4c3c-be0c-359bbd97381e"
''')

Path("internal/modules/topic/compatibility_test.go").write_text('''package topic

import (
\t"encoding/json"
\t"testing"
)

func TestTopicResponseContracts(t *testing.T) {
\tmutation, err := json.Marshal(MutationResponse{Object: ObjectTopic, ID: "topic-id"})
\tif err != nil { t.Fatal(err) }
\tif string(mutation) != `{"object":"topic","id":"topic-id"}` { t.Fatalf("unexpected mutation response: %s", mutation) }
\tdeleted, err := json.Marshal(DeleteResponse{Object: ObjectTopic, ID: "topic-id", Deleted: true})
\tif err != nil { t.Fatal(err) }
\tif string(deleted) != `{"object":"topic","id":"topic-id","deleted":true}` { t.Fatalf("unexpected delete response: %s", deleted) }
}

func TestNormalizeTopicAPIListRequest(t *testing.T) {
\trequest := APIListRequest{}
\tif err := normalizeAPIListRequest(&request); err != nil { t.Fatal(err) }
\tif request.Limit != 20 { t.Fatalf("default limit = %d", request.Limit) }
\tif err := normalizeAPIListRequest(&APIListRequest{Limit: -1}); err == nil { t.Fatal("expected invalid limit") }
\tif err := normalizeAPIListRequest(&APIListRequest{After: uuidText, Before: uuidText}); err == nil { t.Fatal("expected mutually exclusive cursors") }
}

func TestCreateTopicDefaultsPrivate(t *testing.T) {
\trequest, err := validateCreate(CreateRequest{Name: "Newsletter", DefaultSubscription: "opt_in"})
\tif err != nil { t.Fatal(err) }
\tif request.Visibility != "private" { t.Fatalf("visibility = %q, want private", request.Visibility) }
}

const uuidText = "b6d24b8e-af0b-4c3c-be0c-359bbd97381e"
''')
