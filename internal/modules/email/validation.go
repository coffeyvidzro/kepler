package email

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

const (
	maxEmailNameCharacters = 128
	maxSubjectCharacters   = 255
	maxBodyBytes           = platformemail.MaxBodyBytes
	maxMetadataBytes       = 16 << 10
	maxRecipients          = 50
	maxAttachmentsBytes    = platformemail.MaxAttachmentsDecodedBytes
)

var tagPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type validatedSend struct {
	SenderDomainID *uuid.UUID
	Provider       string
	ProviderRegion string
	DeliveryRoute  platformemail.DeliveryRoute
	MessageType    string
	FromEmail      string
	FromName       *string
	ReplyToEmail   *string
	To             []EmailAddress
	CC             []EmailAddress
	BCC            []EmailAddress
	ReplyTo        []EmailAddress
	ToEmail        string
	ToName         *string
	Subject        string
	HTMLBody       *string
	TextBody       *string
	Metadata       json.RawMessage
	Headers        map[string]string
	Attachments    []Attachment
	Tags           []Tag
	ScheduledAt    *time.Time
}

func validateSend(req SendRequest, config ServiceConfig) (validatedSend, error) {
	stream, err := normalizeStream(req.Stream)
	if err != nil {
		return validatedSend{}, err
	}
	defaultFrom, err := normalizeEmail(config.DefaultFromEmail, "Configured email sender")
	if err != nil {
		return validatedSend{}, apperrors.NewInternal("Email sender is not configured", err)
	}

	to, err := normalizeAddresses(req.To, "To", true)
	if err != nil {
		return validatedSend{}, err
	}
	cc, err := normalizeAddresses(req.CC, "Cc", false)
	if err != nil {
		return validatedSend{}, err
	}
	bcc, err := normalizeAddresses(req.BCC, "Bcc", false)
	if err != nil {
		return validatedSend{}, err
	}
	if len(to)+len(cc)+len(bcc) > maxRecipients {
		return validatedSend{}, apperrors.NewBadRequest("Email may have at most 50 recipients")
	}
	toEmail := to[0].Email
	toName := stringPointer(to[0].Name)

	fromEmail := defaultFrom
	fromNameValue := strings.TrimSpace(config.DefaultFromName)
	if req.From != nil {
		if strings.TrimSpace(req.From.Email) != "" {
			requestedFrom, fromErr := normalizeEmail(req.From.Email, "Email sender")
			if fromErr != nil {
				return validatedSend{}, apperrors.NewBadRequest(fromErr.Error())
			}
			fromEmail = requestedFrom
		}
		if strings.TrimSpace(req.From.Name) != "" {
			fromNameValue = strings.TrimSpace(req.From.Name)
		}
	}
	fromName, err := normalizeName(fromNameValue, "Email sender name")
	if err != nil {
		return validatedSend{}, err
	}

	replyToAddresses, err := normalizeAddresses(req.ReplyTo, "Reply-to", false)
	if err != nil {
		return validatedSend{}, err
	}
	var replyTo *string
	if len(replyToAddresses) > 0 {
		replyTo = &replyToAddresses[0].Email
	}

	subject := strings.TrimSpace(req.Subject)
	if subject == "" {
		return validatedSend{}, apperrors.NewBadRequest("Email subject is required")
	}
	if utf8.RuneCountInString(subject) > maxSubjectCharacters {
		return validatedSend{}, apperrors.NewBadRequest(fmt.Sprintf("Email subject must be at most %d characters", maxSubjectCharacters))
	}

	htmlBody := optionalBody(req.HTML)
	textBody := optionalBody(req.Text)
	if htmlBody == nil && textBody == nil {
		return validatedSend{}, apperrors.NewBadRequest("Email HTML or text body is required")
	}
	if htmlBody != nil && len(*htmlBody) > maxBodyBytes {
		return validatedSend{}, apperrors.NewPayloadTooLarge("Email HTML body is too large")
	}
	if textBody != nil && len(*textBody) > maxBodyBytes {
		return validatedSend{}, apperrors.NewPayloadTooLarge("Email text body is too large")
	}

	metadata, err := normalizeMetadata(req.Metadata)
	if err != nil {
		return validatedSend{}, err
	}

	headers, err := normalizeHeaders(req.Headers)
	if err != nil {
		return validatedSend{}, err
	}
	attachments, err := normalizeAttachments(req.Attachments)
	if err != nil {
		return validatedSend{}, err
	}
	tags, err := normalizeTags(req.Tags)
	if err != nil {
		return validatedSend{}, err
	}
	scheduledAt, err := normalizeSchedule(req.ScheduledAt)
	if err != nil {
		return validatedSend{}, err
	}

	return validatedSend{
		MessageType: stream, FromEmail: fromEmail, FromName: fromName,
		ReplyToEmail: replyTo, ToEmail: toEmail, ToName: toName, To: to, CC: cc, BCC: bcc,
		ReplyTo: replyToAddresses, Subject: subject, HTMLBody: htmlBody, TextBody: textBody,
		Metadata: metadata, Headers: headers, Attachments: attachments, Tags: tags, ScheduledAt: scheduledAt,
	}, nil
}

func normalizeStream(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return MessageTypeTransactional, nil
	}
	switch value {
	case MessageTypeTransactional, MessageTypeMarketing:
		return value, nil
	default:
		return "", apperrors.NewBadRequest("Email stream must be transactional or marketing")
	}
}

func normalizeAddresses(values []EmailAddress, label string, required bool) ([]EmailAddress, error) {
	if required && len(values) == 0 {
		return nil, apperrors.NewBadRequest(label + " recipient is required")
	}
	result := make([]EmailAddress, 0, len(values))
	for _, value := range values {
		email, err := normalizeEmail(value.Email, label+" recipient")
		if err != nil {
			return nil, apperrors.NewBadRequest(err.Error())
		}
		name, err := normalizeName(value.Name, label+" recipient name")
		if err != nil {
			return nil, err
		}
		address := EmailAddress{Email: email}
		if name != nil {
			address.Name = *name
		}
		result = append(result, address)
	}
	return result, nil
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func normalizeHeaders(headers map[string]string) (map[string]string, error) {
	if len(headers) > 100 {
		return nil, apperrors.NewBadRequest("Email may have at most 100 custom headers")
	}
	result := make(map[string]string, len(headers))
	for key, value := range headers {
		key = strings.TrimSpace(key)
		if key == "" || strings.ContainsAny(key+value, "\r\n") {
			return nil, apperrors.NewBadRequest("Email headers must not be empty or contain newlines")
		}
		result[key] = value
	}
	return result, nil
}

func normalizeAttachments(items []Attachment) ([]Attachment, error) {
	if items == nil {
		items = []Attachment{}
	}
	total := 0
	for i := range items {
		items[i].Filename = strings.TrimSpace(items[i].Filename)
		if items[i].Filename == "" {
			return nil, apperrors.NewBadRequest("Attachment filename is required")
		}
		if (items[i].Content == "") == (items[i].Path == "") {
			return nil, apperrors.NewBadRequest("Attachment must provide exactly one of content or path")
		}
		if items[i].Path != "" {
			return nil, apperrors.NewBadRequest("Attachment paths are not supported; provide Base64 content")
		}
		decodedSize, err := attachmentContentSize(items[i].Content)
		if err != nil {
			return nil, apperrors.NewBadRequest("Attachment content must be valid Base64")
		}
		total += decodedSize
	}
	if total > maxAttachmentsBytes {
		return nil, apperrors.NewPayloadTooLarge("Email attachments exceed 7MB")
	}
	return items, nil
}

func attachmentContentSize(content string) (int, error) {
	decoded, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		return 0, err
	}
	return len(decoded), nil
}

func normalizeTags(tags []Tag) ([]Tag, error) {
	if tags == nil {
		tags = []Tag{}
	}
	for i := range tags {
		tags[i].Name, tags[i].Value = strings.TrimSpace(tags[i].Name), strings.TrimSpace(tags[i].Value)
		if len(tags[i].Name) > 256 || len(tags[i].Value) > 256 || !tagPattern.MatchString(tags[i].Name) || !tagPattern.MatchString(tags[i].Value) {
			return nil, apperrors.NewBadRequest("Email tag names and values must use letters, numbers, underscores, or dashes and be at most 256 characters")
		}
	}
	return tags, nil
}

func normalizeSchedule(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	when, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		parts := strings.Fields(strings.ToLower(value))
		if len(parts) == 3 && parts[0] == "in" {
			n, numberErr := strconv.Atoi(parts[1])
			if numberErr == nil && n > 0 {
				units := map[string]time.Duration{"second": time.Second, "seconds": time.Second, "sec": time.Second, "minute": time.Minute, "minutes": time.Minute, "min": time.Minute, "hour": time.Hour, "hours": time.Hour, "day": 24 * time.Hour, "days": 24 * time.Hour}
				if unit, ok := units[parts[2]]; ok {
					candidate := time.Now().UTC().Add(time.Duration(n) * unit)
					when = candidate
					err = nil
				}
			}
		}
	}
	if err != nil || !when.After(time.Now().UTC()) {
		return nil, apperrors.NewBadRequest("scheduled_at must be a future ISO 8601 time or a value such as 'in 5 min'")
	}
	when = when.UTC()
	return &when, nil
}

func normalizeUpdateSchedule(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, apperrors.NewBadRequest("scheduled_at is required")
	}
	when, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || !when.After(time.Now().UTC()) {
		return time.Time{}, apperrors.NewBadRequest("scheduled_at must be a future ISO 8601 time")
	}
	return when.UTC(), nil
}

func normalizeEmail(value string, label string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	parsed, err := mail.ParseAddress(value)
	if err != nil || !strings.EqualFold(parsed.Address, value) {
		return "", fmt.Errorf("%s must be a valid email address", label)
	}
	return strings.ToLower(parsed.Address), nil
}

func normalizeName(value string, label string) (*string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(value) > maxEmailNameCharacters {
		return nil, apperrors.NewBadRequest(fmt.Sprintf("%s must be at most %d characters", label, maxEmailNameCharacters))
	}
	return &value, nil
}

func optionalBody(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func normalizeMetadata(metadata json.RawMessage) (json.RawMessage, error) {
	if len(metadata) == 0 {
		return json.RawMessage(`{}`), nil
	}
	if len(metadata) > maxMetadataBytes {
		return nil, apperrors.NewPayloadTooLarge("Email metadata is too large")
	}
	var object map[string]any
	if err := json.Unmarshal(metadata, &object); err != nil || object == nil {
		return nil, apperrors.NewBadRequest("Email metadata must be a JSON object")
	}
	canonical, err := json.Marshal(object)
	if err != nil {
		return nil, apperrors.NewBadRequest("Email metadata must be valid JSON")
	}
	if len(canonical) > maxMetadataBytes {
		return nil, apperrors.NewPayloadTooLarge("Email metadata is too large")
	}
	return canonical, nil
}
