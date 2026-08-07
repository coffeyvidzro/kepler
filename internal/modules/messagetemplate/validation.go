package messagetemplate

import (
	"encoding/json"
	"net/mail"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

const (
	maxAPIPerPage              = 100
	maxTemplateNameCharacters  = 100
	maxTemplateAliasCharacters = 100
	maxTemplateVariables       = 50
	maxTemplateSubjectChars    = 255
)

var (
	aliasPattern       = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	variableKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,49}$`)
	reservedVariables  = map[string]struct{}{
		"FIRST_NAME": {}, "LAST_NAME": {}, "EMAIL": {}, "UNSUBSCRIBE_URL": {}, "RESEND_UNSUBSCRIBE_URL": {},
		"CONTACT": {}, "THIS": {},
	}
)

func validateCreate(req CreateRequest) (CreateRequest, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Alias = normalizeOptional(req.Alias)
	req.Subject = strings.TrimSpace(req.Subject)
	if req.Name == "" || utf8.RuneCountInString(req.Name) > maxTemplateNameCharacters {
		return req, apperrors.NewBadRequest("Template name is required and must be at most 100 characters")
	}
	if err := validateAlias(req.Alias); err != nil {
		return req, err
	}
	if err := validateContent(req.Subject, req.HTML, req.FromEmail, req.ReplyTo, req.Variables); err != nil {
		return req, err
	}
	req.FromEmail = normalizeOptional(req.FromEmail)
	req.FromName = normalizeOptional(req.FromName)
	return req, nil
}

func validateUpdate(template Template, base Version, req *UpdateRequest) error {
	name, alias := template.Name, template.Alias
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
		req.Name = &name
	}
	if req.Alias != nil {
		alias = normalizeOptional(*req.Alias)
		req.Alias = &alias
	}
	if name == "" || utf8.RuneCountInString(name) > maxTemplateNameCharacters {
		return apperrors.NewBadRequest("Template name is required and must be at most 100 characters")
	}
	if err := validateAlias(alias); err != nil {
		return err
	}
	subject, htmlBody, variables := base.Subject, base.HTML, base.Variables
	fromEmail, replyTo := base.FromEmail, base.ReplyToEmail
	if req.Subject != nil {
		subject = strings.TrimSpace(*req.Subject)
		req.Subject = &subject
	}
	if req.HTML != nil {
		htmlBody = *req.HTML
	}
	if req.Variables != nil {
		variables = *req.Variables
	}
	if req.FromEmail != nil {
		fromEmail = *req.FromEmail
	}
	if req.ReplyTo != nil {
		replyTo = *req.ReplyTo
	}
	return validateContent(subject, htmlBody, fromEmail, replyTo, variables)
}

func validateVersion(version Version) error {
	if strings.TrimSpace(version.Subject) == "" {
		return apperrors.NewBadRequest("Template subject is required before publishing")
	}
	return validateContent(version.Subject, version.HTML, version.FromEmail, version.ReplyToEmail, version.Variables)
}

func validateAlias(alias *string) error {
	if alias == nil {
		return nil
	}
	if len(*alias) > maxTemplateAliasCharacters || !aliasPattern.MatchString(*alias) {
		return apperrors.NewBadRequest("Template alias must use letters, numbers, underscores, or dashes and be at most 100 characters")
	}
	return nil
}

func validateContent(subject, htmlBody string, fromEmail, replyTo *string, variables []Variable) error {
	if utf8.RuneCountInString(subject) > maxTemplateSubjectChars {
		return apperrors.NewBadRequest("Template subject must be at most 255 characters")
	}
	if strings.TrimSpace(htmlBody) == "" {
		return apperrors.NewBadRequest("Template HTML is required")
	}
	if len(variables) > maxTemplateVariables {
		return apperrors.NewBadRequest("Template may define at most 50 variables")
	}
	definitions := map[string]struct{}{}
	for _, variable := range variables {
		if !variableKeyPattern.MatchString(variable.Key) {
			return apperrors.NewBadRequest("Template variable keys must start with a letter and use only letters, numbers, and underscores")
		}
		upper := strings.ToUpper(variable.Key)
		if _, reserved := reservedVariables[upper]; reserved {
			return apperrors.NewBadRequest("Template variable key is reserved: " + variable.Key)
		}
		if _, exists := definitions[variable.Key]; exists {
			return apperrors.NewBadRequest("Template variable keys must be unique")
		}
		definitions[variable.Key] = struct{}{}
		if variable.Type != VariableTypeString && variable.Type != VariableTypeNumber {
			return apperrors.NewBadRequest("Template variable type must be string or number")
		}
		if variable.FallbackValue != nil {
			if _, err := renderVariableValue(variable, variable.FallbackValue); err != nil {
				return apperrors.NewBadRequest(err.Error())
			}
		}
	}
	for _, key := range referencedVariables(subject, htmlBody) {
		if _, exists := definitions[key]; !exists {
			return apperrors.NewBadRequest("Unknown template variable: " + key)
		}
	}
	if err := validateStoredEmail(fromEmail, "Template sender"); err != nil {
		return err
	}
	if replyTo != nil {
		values, err := decodeStoredReplyTo(replyTo)
		if err != nil {
			return apperrors.NewBadRequest("Template reply-to addresses are invalid")
		}
		for _, value := range values {
			address, parseErr := mail.ParseAddress(strings.TrimSpace(value))
			if parseErr != nil || address.Address == "" {
				return apperrors.NewBadRequest("Template reply-to addresses are invalid")
			}
		}
	}
	return nil
}

func validateStoredEmail(value *string, label string) error {
	if value == nil {
		return nil
	}
	parsed, err := mail.ParseAddress(strings.TrimSpace(*value))
	if err != nil || !strings.EqualFold(parsed.Address, strings.TrimSpace(*value)) {
		return apperrors.NewBadRequest(label + " must be a valid email address")
	}
	return nil
}

func normalizeOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func normalizeList(req *ListRequest) {
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 50
	}
	if req.Offset < 0 {
		req.Offset = 0
	}
}

func normalizeAPIListRequest(request *APIListRequest) error {
	if request.Limit == 0 {
		request.Limit = 20
	}
	if request.Limit < 1 || request.Limit > maxAPIPerPage {
		return apperrors.NewBadRequest("Limit must be between 1 and 100")
	}
	request.After = strings.TrimSpace(request.After)
	request.Before = strings.TrimSpace(request.Before)
	if request.After != "" && request.Before != "" {
		return apperrors.NewBadRequest("After and before cannot be used together")
	}
	return nil
}

func parseTemplateCursor(value string) (*uuid.UUID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return nil, apperrors.NewBadRequest("Template cursor must be a valid UUID")
	}
	return &id, nil
}

func splitSender(value *string) (*string, *string, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil, nil
	}
	address, err := mail.ParseAddress(strings.TrimSpace(*value))
	if err != nil || address.Address == "" {
		return nil, nil, apperrors.NewBadRequest("From must be a valid email address")
	}
	email := strings.ToLower(address.Address)
	var name *string
	if strings.TrimSpace(address.Name) != "" {
		text := strings.TrimSpace(address.Name)
		name = &text
	}
	return &email, name, nil
}

func normalizeReplyTo(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		address, err := mail.ParseAddress(strings.TrimSpace(value))
		if err != nil || address.Address == "" {
			return nil, apperrors.NewBadRequest("Reply-to must contain valid email addresses")
		}
		address.Address = strings.ToLower(address.Address)
		result = append(result, address.String())
	}
	return result, nil
}

func encodeStoredReplyTo(values []string) (*string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	if len(values) == 1 {
		value := values[0]
		return &value, nil
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	value := string(encoded)
	return &value, nil
}

func decodeStoredReplyTo(value *string) ([]string, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if !strings.HasPrefix(trimmed, "[") {
		return []string{trimmed}, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(trimmed), &values); err != nil {
		return nil, err
	}
	return values, nil
}
