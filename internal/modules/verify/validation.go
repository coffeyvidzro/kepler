package verify

import (
	"bytes"
	"encoding/json"
	"net/mail"
	"regexp"
	"strings"
	"unicode"

	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

const (
	defaultCodeLength            int32 = 6
	defaultTTLSeconds            int32 = 300
	defaultMaxAttempts           int32 = 5
	defaultResendCooldownSeconds int32 = 30
	defaultMaxResends            int32 = 3
	maxMetadataBytes                   = 16 << 10
)

var serviceKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

type validatedService struct {
	Key                   string
	Name                  string
	DefaultChannel        string
	CodeLength            int32
	TTLSeconds            int32
	MaxAttempts           int32
	ResendCooldownSeconds int32
	MaxResends            int32
	Enabled               bool
	Metadata              json.RawMessage
}

type validatedVerification struct {
	ServiceID           string
	ServiceKey          string
	Channel             string
	Recipient           string
	RecipientNormalized string
	Locale              *string
	Metadata            json.RawMessage
}

func validateCreateService(req CreateServiceRequest) (validatedService, error) {
	resendCooldown := defaultResendCooldownSeconds
	if req.ResendCooldownSeconds != nil {
		resendCooldown = *req.ResendCooldownSeconds
	}
	maxResends := defaultMaxResends
	if req.MaxResends != nil {
		maxResends = *req.MaxResends
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	value := validatedService{
		Key:                   strings.ToLower(strings.TrimSpace(req.Key)),
		Name:                  strings.TrimSpace(req.Name),
		DefaultChannel:        defaultString(req.DefaultChannel, ChannelSMS),
		CodeLength:            defaultInt32(req.CodeLength, defaultCodeLength),
		TTLSeconds:            defaultInt32(req.TTLSeconds, defaultTTLSeconds),
		MaxAttempts:           defaultInt32(req.MaxAttempts, defaultMaxAttempts),
		ResendCooldownSeconds: resendCooldown,
		MaxResends:            maxResends,
		Enabled:               enabled,
		Metadata:              req.Metadata,
	}
	if err := validateService(value); err != nil {
		return validatedService{}, err
	}
	return value, nil
}

func validateUpdateService(current VerificationService, req UpdateServiceRequest) (validatedService, error) {
	value := validatedService{
		Key: current.Key, Name: current.Name, DefaultChannel: current.DefaultChannel,
		CodeLength: current.CodeLength, TTLSeconds: current.TTLSeconds, MaxAttempts: current.MaxAttempts,
		ResendCooldownSeconds: current.ResendCooldownSeconds, MaxResends: current.MaxResends,
		Enabled: current.Enabled, Metadata: current.Metadata,
	}
	if req.Name != nil {
		value.Name = strings.TrimSpace(*req.Name)
	}
	if req.DefaultChannel != nil {
		value.DefaultChannel = strings.ToLower(strings.TrimSpace(*req.DefaultChannel))
	}
	if req.CodeLength != nil {
		value.CodeLength = *req.CodeLength
	}
	if req.TTLSeconds != nil {
		value.TTLSeconds = *req.TTLSeconds
	}
	if req.MaxAttempts != nil {
		value.MaxAttempts = *req.MaxAttempts
	}
	if req.ResendCooldownSeconds != nil {
		value.ResendCooldownSeconds = *req.ResendCooldownSeconds
	}
	if req.MaxResends != nil {
		value.MaxResends = *req.MaxResends
	}
	if req.Enabled != nil {
		value.Enabled = *req.Enabled
	}
	if req.Metadata != nil {
		value.Metadata = *req.Metadata
	}
	if err := validateService(value); err != nil {
		return validatedService{}, err
	}
	return value, nil
}

func validateService(value validatedService) error {
	if !serviceKeyPattern.MatchString(value.Key) {
		return apperrors.NewBadRequest("Verification service key must use lowercase letters, numbers, dots, underscores, or hyphens")
	}
	if value.Name == "" || len(value.Name) > 120 {
		return apperrors.NewBadRequest("Verification service name must be between 1 and 120 characters")
	}
	if value.DefaultChannel != ChannelEmail && value.DefaultChannel != ChannelSMS {
		return apperrors.NewBadRequest("Verification service channel must be email or sms")
	}
	if value.CodeLength < 4 || value.CodeLength > 10 {
		return apperrors.NewBadRequest("Verification code length must be between 4 and 10 digits")
	}
	if value.TTLSeconds < 30 || value.TTLSeconds > 3600 {
		return apperrors.NewBadRequest("Verification TTL must be between 30 and 3600 seconds")
	}
	if value.MaxAttempts < 1 || value.MaxAttempts > 20 {
		return apperrors.NewBadRequest("Verification max attempts must be between 1 and 20")
	}
	if value.ResendCooldownSeconds < 0 || value.ResendCooldownSeconds > 3600 {
		return apperrors.NewBadRequest("Verification resend cooldown must be between 0 and 3600 seconds")
	}
	if value.MaxResends < 0 || value.MaxResends > 20 {
		return apperrors.NewBadRequest("Verification max resends must be between 0 and 20")
	}
	_, err := normalizeJSONObject(value.Metadata)
	return err
}

func validateCreateVerification(req CreateVerificationRequest, service VerificationService) (validatedVerification, error) {
	serviceID := strings.TrimSpace(req.ServiceID)
	serviceKey := strings.ToLower(strings.TrimSpace(req.Service))
	if (serviceID == "") == (serviceKey == "") {
		return validatedVerification{}, apperrors.NewBadRequest("Exactly one of service_id or service is required")
	}
	channel := strings.ToLower(strings.TrimSpace(req.Channel))
	if channel == "" {
		channel = service.DefaultChannel
	}
	if channel != ChannelEmail && channel != ChannelSMS {
		return validatedVerification{}, apperrors.NewBadRequest("Verification channel must be email or sms")
	}
	recipient := strings.TrimSpace(req.Recipient)
	if recipient == "" {
		return validatedVerification{}, apperrors.NewBadRequest("Verification recipient is required")
	}
	normalized, err := normalizeRecipient(channel, recipient)
	if err != nil {
		return validatedVerification{}, err
	}
	locale := normalizeOptionalString(req.Locale, 35)
	metadata, err := normalizeJSONObject(req.Metadata)
	if err != nil {
		return validatedVerification{}, err
	}
	return validatedVerification{
		ServiceID: serviceID, ServiceKey: serviceKey, Channel: channel,
		Recipient: recipient, RecipientNormalized: normalized, Locale: locale, Metadata: metadata,
	}, nil
}

func validateCheck(req CheckRequest) (CheckRequest, error) {
	req.Code = strings.TrimSpace(req.Code)
	if len(req.Code) < 4 || len(req.Code) > 10 || strings.IndexFunc(req.Code, func(r rune) bool { return !unicode.IsDigit(r) }) >= 0 {
		return CheckRequest{}, apperrors.NewBadRequest("Verification code has an invalid format")
	}
	metadata, err := normalizeJSONObject(req.Metadata)
	if err != nil {
		return CheckRequest{}, err
	}
	req.Metadata = metadata
	if req.UserAgent != nil {
		value := strings.TrimSpace(*req.UserAgent)
		if len(value) > 512 {
			value = value[:512]
		}
		req.UserAgent = &value
	}
	return req, nil
}

func normalizeRecipient(channel, recipient string) (string, error) {
	if channel == ChannelEmail {
		address, err := mail.ParseAddress(recipient)
		if err != nil || !strings.Contains(address.Address, "@") {
			return "", apperrors.NewBadRequest("Verification recipient must be a valid email address")
		}
		return strings.ToLower(address.Address), nil
	}
	var builder strings.Builder
	for index, character := range recipient {
		switch {
		case character >= '0' && character <= '9':
			builder.WriteRune(character)
		case character == '+' && index == 0:
			builder.WriteRune(character)
		case unicode.IsSpace(character) || strings.ContainsRune("()-.", character):
			continue
		default:
			return "", apperrors.NewBadRequest("Verification recipient must be an E.164 phone number")
		}
	}
	normalized := builder.String()
	if !strings.HasPrefix(normalized, "+") || len(normalized) < 9 || len(normalized) > 16 {
		return "", apperrors.NewBadRequest("Verification recipient must be an E.164 phone number")
	}
	return normalized, nil
}

func normalizeJSONObject(value json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 {
		return json.RawMessage(`{}`), nil
	}
	if len(value) > maxMetadataBytes || !json.Valid(trimmed) || trimmed[0] != '{' {
		return nil, apperrors.NewBadRequest("Metadata must be a JSON object no larger than 16 KiB")
	}
	return value, nil
}

func normalizeOptionalString(value *string, max int) *string {
	if value == nil {
		return nil
	}
	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil
	}
	if len(normalized) > max {
		normalized = normalized[:max]
	}
	return &normalized
}

func defaultString(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	return value
}

func defaultInt32(value, fallback int32) int32 {
	if value == 0 {
		return fallback
	}
	return value
}
