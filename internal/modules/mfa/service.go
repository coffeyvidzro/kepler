package mfa

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/audit"
	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
	notifications "github.com/coffeyvidzro/dugble/server/internal/platform/systemmail"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

const recoveryCodeCount = 10
const loginChallengeTTL = 5 * time.Minute
const loginChallengePrefix = "dgb_mfa_"

type store interface {
	PutUnverified(context.Context, uuid.UUID, []byte) error
	GetCredential(context.Context, uuid.UUID) (Credential, error)
	RotateSecretCiphertext(context.Context, uuid.UUID, []byte, []byte) error
	Confirm(context.Context, uuid.UUID, string, int64, []string) error
	Verify(context.Context, uuid.UUID, string, int64) error
	UseRecoveryCode(context.Context, uuid.UUID, string, string) error
	Disable(context.Context, uuid.UUID, string) error
	Enabled(context.Context, uuid.UUID) (bool, error)
	CreateLoginChallenge(context.Context, string, uuid.UUID, int64, time.Time) error
	GetLoginChallenge(context.Context, string) (uuid.UUID, Credential, error)
	ConsumeLoginTOTP(context.Context, string, uuid.UUID, int64) error
	ConsumeLoginRecoveryCode(context.Context, string, uuid.UUID, string) error
}

type Service struct {
	repository store
	cipher     *authnz.SecretCipher
	issuer     string
	now        func() time.Time
	notifier   SecurityNotifier
	recipients RecipientStore
}

type SecurityNotifier interface {
	SendMFAEnabled(context.Context, notifications.SendSecurityEventInput) error
	SendMFADisabled(context.Context, notifications.SendSecurityEventInput) error
	SendRecoveryCodeUsed(context.Context, notifications.SendSecurityEventInput) error
	SendMFALoginFailed(context.Context, notifications.SendSecurityEventInput) error
}

type RecipientStore interface {
	GetNotificationRecipient(context.Context, uuid.UUID) (notifications.Recipient, error)
}

func NewService(repository store, cipher *authnz.SecretCipher, issuer string) *Service {
	return &Service{repository: repository, cipher: cipher, issuer: issuer, now: time.Now}
}

func (s *Service) WithNotifier(notifier SecurityNotifier) *Service {
	s.notifier = notifier
	return s
}

func (s *Service) WithRecipientStore(recipients RecipientStore) *Service {
	s.recipients = recipients
	return s
}

func (s *Service) Enroll(ctx context.Context) (EnrollResponse, error) {
	principal, err := principalFromContext(ctx)
	if err != nil {
		return EnrollResponse{}, err
	}
	secret, err := authnz.NewTOTPSecret()
	if err != nil {
		return EnrollResponse{}, apperrors.NewInternal("Unable to generate MFA secret", err)
	}
	ciphertext, err := s.cipher.Encrypt([]byte(secret))
	if err != nil {
		return EnrollResponse{}, apperrors.NewInternal("Unable to protect MFA secret", err)
	}
	if err := s.repository.PutUnverified(ctx, principal.UserID, ciphertext); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EnrollResponse{}, apperrors.NewConflict("MFA is already enabled")
		}
		return EnrollResponse{}, apperrors.NewInternal("Unable to begin MFA enrollment", err)
	}
	return EnrollResponse{Secret: secret, URI: authnz.TOTPURI(s.issuer, principal.Email, secret)}, nil
}

func (s *Service) Confirm(ctx context.Context, code string) (ConfirmResponse, error) {
	principal, err := principalFromContext(ctx)
	if err != nil {
		return ConfirmResponse{}, err
	}
	credential, err := s.repository.GetCredential(ctx, principal.UserID)
	if err != nil || credential.VerifiedAt != nil {
		return ConfirmResponse{}, apperrors.NewBadRequest("MFA enrollment is not pending")
	}
	step, ok := s.validateCredential(ctx, principal.UserID, credential, code)
	if !ok {
		return ConfirmResponse{}, apperrors.NewUnauthorized("Invalid authentication code")
	}
	codes, hashes, err := newRecoveryCodes()
	if err != nil {
		return ConfirmResponse{}, apperrors.NewInternal("Unable to create recovery codes", err)
	}
	if err := s.repository.Confirm(ctx, principal.UserID, principal.SessionID, step, hashes); err != nil {
		return ConfirmResponse{}, apperrors.NewInternal("Unable to confirm MFA enrollment", err)
	}
	audit.RecordIdentity(ctx, principal.UserID, audit.Event{Action: "identity.mfa_enabled", ResourceType: "user", ResourceID: principal.UserID.String()})
	s.notify(ctx, principal, "enabled")
	return ConfirmResponse{RecoveryCodes: codes}, nil
}

func (s *Service) Verify(ctx context.Context, code string) error {
	principal, err := principalFromContext(ctx)
	if err != nil {
		return err
	}
	credential, err := s.repository.GetCredential(ctx, principal.UserID)
	if err != nil || credential.VerifiedAt == nil {
		return apperrors.NewBadRequest("MFA is not enabled")
	}
	step, ok := s.validateCredential(ctx, principal.UserID, credential, code)
	if !ok {
		return apperrors.NewUnauthorized("Invalid authentication code")
	}
	if credential.LastUsedStep != nil && step <= *credential.LastUsedStep {
		return apperrors.NewUnauthorized("Authentication code was already used")
	}
	if err := s.repository.Verify(ctx, principal.UserID, principal.SessionID, step); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.NewUnauthorized("Authentication code was already used")
		}
		return apperrors.NewInternal("Unable to verify authentication code", err)
	}
	audit.RecordIdentity(ctx, principal.UserID, audit.Event{Action: "identity.mfa_verified", ResourceType: "session", ResourceID: principal.SessionID})
	return nil
}

func (s *Service) Recover(ctx context.Context, code string) error {
	principal, err := principalFromContext(ctx)
	if err != nil {
		return err
	}
	code, err = validateRecoveryCode(code)
	if err != nil {
		return err
	}
	hash := authnz.HashRecoveryCode(code)
	if err := s.repository.UseRecoveryCode(ctx, principal.UserID, principal.SessionID, hash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.NewUnauthorized("Invalid or used recovery code")
		}
		return apperrors.NewInternal("Unable to use recovery code", err)
	}
	audit.RecordIdentity(ctx, principal.UserID, audit.Event{Action: "identity.recovery_code_used", ResourceType: "session", ResourceID: principal.SessionID})
	s.notify(ctx, principal, "recovery")
	return nil
}

func (s *Service) Disable(ctx context.Context) error {
	principal, err := principalFromContext(ctx)
	if err != nil {
		return err
	}
	if err := s.repository.Disable(ctx, principal.UserID, principal.SessionID); err != nil {
		return apperrors.NewInternal("Unable to disable MFA", err)
	}
	audit.RecordIdentity(ctx, principal.UserID, audit.Event{Action: "identity.mfa_disabled", ResourceType: "user", ResourceID: principal.UserID.String()})
	s.notify(ctx, principal, "disabled")
	return nil
}

func (s *Service) notify(ctx context.Context, principal authnz.Principal, event string) {
	if s.notifier == nil || strings.TrimSpace(principal.Email) == "" {
		return
	}
	input := notifications.SendSecurityEventInput{ToEmail: principal.Email, Name: principal.Name}
	var err error
	switch event {
	case "enabled":
		err = s.notifier.SendMFAEnabled(ctx, input)
	case "disabled":
		err = s.notifier.SendMFADisabled(ctx, input)
	case "recovery":
		err = s.notifier.SendRecoveryCodeUsed(ctx, input)
	}
	if err != nil {
		slog.Warn("failed to send MFA security notification", "error", err, "event", event, "user_id", principal.UserID)
	}
}

func (s *Service) Status(ctx context.Context) (StatusResponse, error) {
	principal, err := principalFromContext(ctx)
	if err != nil {
		return StatusResponse{}, err
	}
	enabled, err := s.repository.Enabled(ctx, principal.UserID)
	if err != nil {
		return StatusResponse{}, apperrors.NewInternal("Unable to get MFA status", err)
	}
	return StatusResponse{Enabled: enabled}, nil
}

func (s *Service) validateCredential(ctx context.Context, userID uuid.UUID, credential Credential, code string) (int64, bool) {
	secret, replacement, rotated, err := s.cipher.DecryptAndRotate(credential.SecretCiphertext)
	if err != nil {
		return 0, false
	}
	if rotated {
		_ = s.repository.RotateSecretCiphertext(ctx, userID, credential.SecretCiphertext, replacement)
	}
	return authnz.ValidateTOTP(string(secret), normalizeAuthenticationCode(code), s.now().UTC())
}

func principalFromContext(ctx context.Context) (authnz.Principal, error) {
	principal, ok := authnz.PrincipalFromContext(ctx)
	if !ok {
		return authnz.Principal{}, apperrors.NewUnauthorized("Authentication is required")
	}
	return principal, nil
}

func newRecoveryCodes() ([]string, []string, error) {
	codes := make([]string, 0, recoveryCodeCount)
	hashes := make([]string, 0, recoveryCodeCount)
	for range recoveryCodeCount {
		code, err := authnz.NewRecoveryCode()
		if err != nil {
			return nil, nil, err
		}
		codes = append(codes, code)
		hashes = append(hashes, authnz.HashRecoveryCode(code))
	}
	return codes, hashes, nil
}

func (s *Service) LoginEnabled(ctx context.Context, userID uuid.UUID) (bool, error) {
	return s.repository.Enabled(ctx, userID)
}

func (s *Service) BeginLogin(ctx context.Context, userID uuid.UUID, credentialVersion int64) (string, error) {
	value, err := authnz.NewSessionToken()
	if err != nil {
		return "", err
	}
	token := loginChallengePrefix + value
	if err := s.repository.CreateLoginChallenge(ctx, authnz.HashSessionToken(token), userID, credentialVersion, s.now().UTC().Add(loginChallengeTTL)); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Service) CompleteLoginTOTP(ctx context.Context, challengeToken, code string) (uuid.UUID, error) {
	tokenHash, err := validateLoginChallengeToken(challengeToken)
	if err != nil {
		return uuid.Nil, err
	}
	userID, credential, err := s.repository.GetLoginChallenge(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, pgx.ErrNoRows
		}
		return uuid.Nil, err
	}
	step, ok := s.validateCredential(ctx, userID, credential, code)
	if !ok || (credential.LastUsedStep != nil && step <= *credential.LastUsedStep) {
		s.notifyFailedLogin(ctx, userID)
		return uuid.Nil, pgx.ErrNoRows
	}
	if err := s.repository.ConsumeLoginTOTP(ctx, tokenHash, userID, step); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.notifyFailedLogin(ctx, userID)
		}
		return uuid.Nil, err
	}
	return userID, nil
}

func (s *Service) CompleteLoginRecovery(ctx context.Context, challengeToken, code string) (uuid.UUID, error) {
	tokenHash, err := validateLoginChallengeToken(challengeToken)
	if err != nil {
		return uuid.Nil, pgx.ErrNoRows
	}
	code, err = validateRecoveryCode(code)
	if err != nil {
		return uuid.Nil, pgx.ErrNoRows
	}
	userID, _, err := s.repository.GetLoginChallenge(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, pgx.ErrNoRows
		}
		return uuid.Nil, err
	}
	if err := s.repository.ConsumeLoginRecoveryCode(ctx, tokenHash, userID, authnz.HashRecoveryCode(code)); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.notifyFailedLogin(ctx, userID)
		}
		return uuid.Nil, err
	}
	return userID, nil
}

func (s *Service) notifyFailedLogin(ctx context.Context, userID uuid.UUID) {
	if s.notifier == nil || s.recipients == nil {
		return
	}
	recipient, err := s.recipients.GetNotificationRecipient(ctx, userID)
	if err != nil {
		slog.Warn("failed to resolve failed MFA notification recipient", "error", err, "user_id", userID)
		return
	}
	if err := s.notifier.SendMFALoginFailed(ctx, notifications.SendSecurityEventInput{ToEmail: recipient.Email, Name: recipient.Name}); err != nil {
		slog.Warn("failed to send failed MFA notification", "error", err, "user_id", userID)
	}
}
