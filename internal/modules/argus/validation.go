package argus

import (
	"bytes"
	"encoding/json"
	"net/mail"
	"strings"
	"time"
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

type validatedVerification struct {
	Channel               string
	Recipient             string
	RecipientNormalized   string
	CodeLength            int32
	TTLSeconds            int32
	MaxAttempts           int32
	ResendCooldownSeconds int32
	MaxResends            int32
	Locale                *string
	Metadata              json.RawMessage
}

func validateCreateVerification(req CreateVerificationRequest) (validatedVerification, error) {
	channel := strings.ToLower(strings.TrimSpace(req.Channel))
	if channel == "" {
		channel = ChannelSMS
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

	codeLength := defaultInt32Pointer(req.CodeLength, defaultCodeLength)
	if codeLength < 4 || codeLength > 10 {
		return validatedVerification{}, apperrors.NewBadRequest("Verification code length must be between 4 and 10 digits")
	}
	ttlSeconds := defaultInt32Pointer(req.TTLSeconds, defaultTTLSeconds)
	if ttlSeconds < 30 || ttlSeconds > 3600 {
		return validatedVerification{}, apperrors.NewBadRequest("Verification TTL must be between 30 and 3600 seconds")
	}
	maxAttempts := defaultInt32Pointer(req.MaxAttempts, defaultMaxAttempts)
	if maxAttempts < 1 || maxAttempts > 20 {
		return validatedVerification{}, apperrors.NewBadRequest("Verification max attempts must be between 1 and 20")
	}
	maxResends := defaultInt32Pointer(req.MaxResends, defaultMaxResends)
	if maxResends < 0 || maxResends > 20 {
		return validatedVerification{}, apperrors.NewBadRequest("Verification max resends must be between 0 and 20")
	}

	metadata, err := normalizeJSONObject(req.Metadata)
	if err != nil {
		return validatedVerification{}, err
	}
	return validatedVerification{
		Channel: channel, Recipient: recipient, RecipientNormalized: normalized,
		CodeLength: codeLength, TTLSeconds: ttlSeconds, MaxAttempts: maxAttempts,
		ResendCooldownSeconds: defaultResendCooldownSeconds, MaxResends: maxResends,
		Locale: normalizeOptionalString(req.Locale, 35), Metadata: metadata,
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

func defaultInt32Pointer(value *int32, fallback int32) int32 {
	if value == nil {
		return fallback
	}
	return *value
}

func terminalCheckResponse(current Verification) CheckResponse {
	return CheckResponse{
		ID:      current.ID,
		Status:  current.Status,
		Valid:   false,
		Expired: current.Status == StatusExpired,
	}
}

func validateResendVerification(current Verification, now time.Time) error {
	if current.Status != StatusPending {
		return apperrors.NewConflict("Only pending verifications can be resent")
	}
	if current.ResendCount >= current.MaxResends {
		return apperrors.TooManyRequests("Verification resend limit reached")
	}
	if !current.ExpiresAt.After(now) {
		return apperrors.NewConflict("Expired verifications cannot be resent")
	}
	return nil
}

func validateResendChallenge(createdAt, expiresAt time.Time, cooldownSeconds int32, now time.Time) error {
	if !expiresAt.After(now) {
		return apperrors.NewConflict("Expired verifications cannot be resent")
	}
	nextAllowed := createdAt.Add(time.Duration(cooldownSeconds) * time.Second)
	if now.Before(nextAllowed) {
		return apperrors.TooManyRequests("Verification resend cooldown is still active")
	}
	return nil
}
