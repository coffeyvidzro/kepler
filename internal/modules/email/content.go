package email

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/awsses"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

var resendTemplatePlaceholder = regexp.MustCompile(`\{\{\{\s*([A-Za-z][A-Za-z0-9_]*)\s*\}\}\}`)

type emailTemplateVariable struct {
	Key           string `json:"key"`
	Type          string `json:"type"`
	FallbackValue any    `json:"fallback_value,omitempty"`
}

type publishedEmailTemplate struct {
	FromEmail *string
	FromName  *string
	ReplyTo   *string
	Subject   string
	HTML      string
	Text      *string
	Variables []emailTemplateVariable
}

func (s *Service) resolveRequestContent(ctx context.Context, teamID uuid.UUID, request SendRequest) (SendRequest, error) {
	if request.Template != nil {
		resolved, err := s.renderPublishedTemplate(ctx, teamID, *request.Template)
		if err != nil {
			return request, err
		}
		if strings.TrimSpace(request.HTML) != "" || strings.TrimSpace(request.Text) != "" {
			return request, apperrors.NewBadRequest("html and text cannot be used when template is provided")
		}
		request.HTML = resolved.HTML
		if resolved.Text != nil {
			request.Text = *resolved.Text
		}
		if strings.TrimSpace(request.Subject) == "" {
			request.Subject = resolved.Subject
		}
		if request.From == nil && resolved.FromEmail != nil {
			request.From = &EmailAddress{Email: *resolved.FromEmail}
			if resolved.FromName != nil {
				request.From.Name = *resolved.FromName
			}
		}
		if len(request.ReplyTo) == 0 && resolved.ReplyTo != nil {
			request.ReplyTo = EmailAddressList{{Email: *resolved.ReplyTo}}
		}
		request.Template = nil
	}
	if strings.TrimSpace(request.TopicID) != "" {
		if err := s.repository.ValidateTopic(ctx, teamID, request.TopicID); err != nil {
			if errors.Is(err, ErrNotFound) {
				return request, apperrors.NewNotFound("Topic not found")
			}
			return request, apperrors.NewInternal("Unable to validate email topic", err)
		}
	}
	attachments, err := resolveRemoteAttachments(ctx, request.Attachments)
	if err != nil {
		return request, err
	}
	request.Attachments = attachments
	return request, nil
}

func (s *Service) renderPublishedTemplate(ctx context.Context, teamID uuid.UUID, reference TemplateReference) (publishedEmailTemplate, error) {
	identifier := strings.TrimSpace(reference.ID)
	if identifier == "" {
		return publishedEmailTemplate{}, apperrors.NewBadRequest("template.id is required")
	}
	value, err := s.repository.GetPublishedTemplate(ctx, teamID, identifier)
	if errors.Is(err, ErrNotFound) {
		return publishedEmailTemplate{}, apperrors.NewNotFound("Published template not found")
	}
	if err != nil {
		return publishedEmailTemplate{}, apperrors.NewInternal("Unable to load published template", err)
	}
	values := make(map[string]string, len(value.Variables))
	definitions := make(map[string]emailTemplateVariable, len(value.Variables))
	for _, definition := range value.Variables {
		definitions[definition.Key] = definition
		raw, exists := reference.Variables[definition.Key]
		if !exists {
			raw = definition.FallbackValue
		}
		if raw == nil {
			continue
		}
		normalized, normalizeErr := normalizeTemplateValue(definition, raw)
		if normalizeErr != nil {
			return publishedEmailTemplate{}, apperrors.NewBadRequest(normalizeErr.Error())
		}
		values[definition.Key] = normalized
	}
	render := func(input string, escape bool) (string, error) {
		var renderErr error
		output := resendTemplatePlaceholder.ReplaceAllStringFunc(input, func(match string) string {
			parts := resendTemplatePlaceholder.FindStringSubmatch(match)
			key := parts[1]
			if _, exists := definitions[key]; !exists {
				renderErr = fmt.Errorf("unknown template variable %s", key)
				return match
			}
			text, exists := values[key]
			if !exists {
				renderErr = fmt.Errorf("template variable %s is required", key)
				return match
			}
			if escape {
				return html.EscapeString(text)
			}
			return text
		})
		return output, renderErr
	}
	value.Subject, err = render(value.Subject, false)
	if err != nil {
		return publishedEmailTemplate{}, apperrors.NewBadRequest(err.Error())
	}
	value.HTML, err = render(value.HTML, true)
	if err != nil {
		return publishedEmailTemplate{}, apperrors.NewBadRequest(err.Error())
	}
	if value.Text != nil {
		text, renderErr := render(*value.Text, false)
		if renderErr != nil {
			return publishedEmailTemplate{}, apperrors.NewBadRequest(renderErr.Error())
		}
		value.Text = &text
	}
	return value, nil
}

func normalizeTemplateValue(definition emailTemplateVariable, value any) (string, error) {
	switch definition.Type {
	case "string":
		text, ok := value.(string)
		if !ok {
			return "", fmt.Errorf("template variable %s must be a string", definition.Key)
		}
		if len(text) > 2000 {
			return "", fmt.Errorf("template variable %s must be at most 2000 characters", definition.Key)
		}
		return text, nil
	case "number":
		switch number := value.(type) {
		case json.Number:
			return number.String(), nil
		case float64:
			if number > 9007199254740991 || number < -9007199254740991 {
				return "", fmt.Errorf("template variable %s exceeds the supported numeric range", definition.Key)
			}
			return strconv.FormatFloat(number, 'f', -1, 64), nil
		case int:
			return strconv.Itoa(number), nil
		case int64:
			return strconv.FormatInt(number, 10), nil
		default:
			return "", fmt.Errorf("template variable %s must be a number", definition.Key)
		}
	default:
		return "", fmt.Errorf("unsupported template variable type %q", definition.Type)
	}
}

func (r *Repository) GetPublishedTemplate(ctx context.Context, teamID uuid.UUID, identifier string) (publishedEmailTemplate, error) {
	var value publishedEmailTemplate
	var variables []byte
	id, parseErr := uuid.Parse(identifier)
	query := `
		SELECT version.from_email, version.from_name, version.reply_to_email,
		       version.subject, version.html_body, version.text_body, version.variables
		FROM message_templates AS template
		JOIN message_template_versions AS version
		  ON version.id = template.published_version_id
		 AND version.template_id = template.id
		 AND version.team_id = template.team_id
		WHERE template.team_id = $1 AND template.deleted_at IS NULL
		  AND lower(template.alias) = lower($2)`
	args := []any{teamID, identifier}
	if parseErr == nil {
		query = strings.ReplaceAll(query, "lower(template.alias) = lower($2)", "template.id = $2")
		args = []any{teamID, id}
	}
	err := r.db.QueryRow(ctx, query, args...).Scan(
		&value.FromEmail, &value.FromName, &value.ReplyTo, &value.Subject,
		&value.HTML, &value.Text, &variables,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return publishedEmailTemplate{}, ErrNotFound
	}
	if err != nil {
		return publishedEmailTemplate{}, err
	}
	if err := json.Unmarshal(variables, &value.Variables); err != nil {
		return publishedEmailTemplate{}, err
	}
	return value, nil
}

func (r *Repository) ValidateTopic(ctx context.Context, teamID uuid.UUID, value string) error {
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return ErrNotFound
	}
	var exists bool
	if err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM topics WHERE id=$1 AND team_id=$2)`, id, teamID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

func resolveRemoteAttachments(ctx context.Context, attachments []Attachment) ([]Attachment, error) {
	if attachments == nil {
		return []Attachment{}, nil
	}
	result := make([]Attachment, len(attachments))
	copy(result, attachments)
	for index := range result {
		if strings.TrimSpace(result[index].Path) == "" {
			continue
		}
		if strings.TrimSpace(result[index].Content) != "" {
			return nil, apperrors.NewBadRequest("Attachment must provide exactly one of content or path")
		}
		content, contentType, err := fetchRemoteAttachment(ctx, result[index].Path)
		if err != nil {
			return nil, err
		}
		result[index].Content = base64.StdEncoding.EncodeToString(content)
		result[index].Path = ""
		if strings.TrimSpace(result[index].ContentType) == "" {
			result[index].ContentType = contentType
		}
	}
	return result, nil
}

func fetchRemoteAttachment(ctx context.Context, rawURL string) ([]byte, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Hostname() == "" || parsed.User != nil {
		return nil, "", apperrors.NewBadRequest("Attachment path must be a valid HTTP or HTTPS URL")
	}
	if err := validatePublicHost(ctx, parsed.Hostname()); err != nil {
		return nil, "", err
	}
	client := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many attachment redirects")
			}
			return validatePublicHost(request.Context(), request.URL.Hostname())
		},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, "", apperrors.NewBadRequest("Attachment path is invalid")
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, "", apperrors.NewBadRequest("Unable to download attachment path")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, "", apperrors.NewBadRequest("Attachment path did not return a successful response")
	}
	maxDecoded := int64(platformemail.MaxAttachmentsEncodedBytes/4*3 + 1)
	content, err := io.ReadAll(io.LimitReader(response.Body, maxDecoded+1))
	if err != nil {
		return nil, "", apperrors.NewBadRequest("Unable to read attachment path")
	}
	if int64(len(content)) > maxDecoded || base64.StdEncoding.EncodedLen(len(content)) > platformemail.MaxAttachmentsEncodedBytes {
		return nil, "", apperrors.NewPayloadTooLarge("Email attachments exceed 40MB after Base64 encoding")
	}
	contentType := strings.TrimSpace(response.Header.Get("Content-Type"))
	if mediaType, _, parseErr := mime.ParseMediaType(contentType); parseErr == nil {
		contentType = mediaType
	} else {
		contentType = mime.TypeByExtension(filepath.Ext(parsed.Path))
	}
	if contentType == "" {
		contentType = http.DetectContentType(content)
	}
	return content, contentType, nil
}

func validatePublicHost(ctx context.Context, hostname string) error {
	if strings.EqualFold(hostname, "localhost") {
		return apperrors.NewBadRequest("Attachment path must use a public host")
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, hostname)
	if err != nil || len(addresses) == 0 {
		return apperrors.NewBadRequest("Attachment path host could not be resolved")
	}
	for _, address := range addresses {
		ip := address.IP
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return apperrors.NewBadRequest("Attachment path must use a public host")
		}
	}
	return nil
}
