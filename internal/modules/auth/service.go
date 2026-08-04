package auth

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/coffeyvidzro/dugble/server/internal/modules/session"
	"github.com/coffeyvidzro/dugble/server/internal/platform/audit"
	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
	notifications "github.com/coffeyvidzro/dugble/server/internal/platform/systemmail"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

const (
	sessionTTL                = 30 * 24 * time.Hour
	emailVerificationTokenTTL = 24 * time.Hour
	passwordResetTokenTTL     = 30 * time.Minute
)

type Service struct {
	repository *Repository
	sessions   *session.Repository
	notifier   IdentityNotifier
	mfa        LoginMFA
}

type LoginMFA interface {
	LoginEnabled(context.Context, uuid.UUID) (bool, error)
	BeginLogin(context.Context, uuid.UUID, int64) (string, error)
	CompleteLoginTOTP(context.Context, string, string) (uuid.UUID, error)
	CompleteLoginRecovery(context.Context, string, string) (uuid.UUID, error)
}

type IdentityNotifier interface {
	SendEmailVerification(ctx context.Context, input notifications.SendEmailVerificationInput) error
	SendPasswordReset(ctx context.Context, input notifications.SendPasswordResetInput) error
}

func NewService(
	repository *Repository,
	sessions *session.Repository,
	notifier IdentityNotifier,
	mfa ...LoginMFA,
) *Service {
	service := &Service{repository: repository, sessions: sessions, notifier: notifier}
	if len(mfa) > 0 {
		service.mfa = mfa[0]
	}
	return service
}

func (s *Service) GetUser(ctx context.Context) (AuthResponse, error) {
	principal, ok := authnz.PrincipalFromContext(ctx)
	if !ok {
		return AuthResponse{}, apperrors.NewUnauthorized("Authentication is required")
	}
	user, err := s.repository.GetUserByID(ctx, principal.UserID)
	if err != nil {
		return AuthResponse{}, apperrors.NewNotFound("User not found")
	}
	return AuthResponse{User: authenticatedUserFromRecord(user)}, nil
}

func (s *Service) Register(
	ctx context.Context,
	req RegisterRequest,
) (AuthResponse, error) {
	email, name, password, err := validateCredentials(req.Email, req.Name, req.Password)
	if err != nil {
		return AuthResponse{}, err
	}

	passwordHash, err := authnz.HashPassword(password)
	if err != nil {
		return AuthResponse{}, apperrors.NewInternal(
			"Unable to hash password",
			err,
		)
	}

	created, err := s.repository.CreateUser(ctx, name, email, passwordHash)
	if err != nil {
		return AuthResponse{}, apperrors.NewInternal(
			"Unable to register user",
			err,
		)
	}

	if err := s.issueEmailVerificationToken(ctx, created.Email, created.Name); err != nil {
		return AuthResponse{}, err
	}

	return AuthResponse{User: authenticatedUserFromRecord(created)}, nil
}

func (s *Service) Login(
	ctx context.Context,
	req LoginRequest,
	userAgent *string,
	ipAddress *string,
) (LoginResponse, string, time.Time, error) {
	email, password, validationErr := validateLoginRequest(req)
	if validationErr != nil {
		return LoginResponse{}, "", time.Time{}, validationErr
	}

	user, err := s.repository.GetUserByEmail(ctx, email)
	if err != nil || user.PasswordHash == nil ||
		!authnz.CheckPassword(*user.PasswordHash, password) {
		return LoginResponse{}, "", time.Time{}, apperrors.NewUnauthorized(
			"Invalid email or password",
		)
	}
	if !user.EmailVerified {
		return LoginResponse{}, "", time.Time{}, apperrors.NewForbidden(
			"Email verification is required",
		)
	}

	if s.mfa != nil {
		enabled, err := s.mfa.LoginEnabled(ctx, user.ID)
		if err != nil {
			return LoginResponse{}, "", time.Time{}, apperrors.NewInternal("Unable to determine MFA requirements", err)
		}
		if enabled {
			challenge, err := s.mfa.BeginLogin(ctx, user.ID, user.CredentialVersion)
			if err != nil {
				return LoginResponse{}, "", time.Time{}, apperrors.NewInternal("Unable to begin MFA login", err)
			}
			return LoginResponse{MFARequired: true, ChallengeToken: challenge, Methods: []string{"totp", "recovery_code"}}, "", time.Time{}, nil
		}
	}
	response, token, expiresAt, err := s.createSession(ctx, user, userAgent, ipAddress, authnz.AuthenticationMethodPassword, authnz.AssuranceLevelOne, nil)
	if err != nil {
		return LoginResponse{}, "", time.Time{}, err
	}
	return LoginResponse{User: &response.User}, token, expiresAt, nil
}

func (s *Service) CompleteMFALogin(ctx context.Context, request MFALoginRequest, recovery bool, userAgent, ipAddress *string) (LoginResponse, string, time.Time, error) {
	if s.mfa == nil {
		return LoginResponse{}, "", time.Time{}, apperrors.NewInternal("MFA login is not configured", nil)
	}
	var userID uuid.UUID
	var err error
	if recovery {
		userID, err = s.mfa.CompleteLoginRecovery(ctx, request.ChallengeToken, request.Code)
	} else {
		userID, err = s.mfa.CompleteLoginTOTP(ctx, request.ChallengeToken, request.Code)
	}
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return LoginResponse{}, "", time.Time{}, apperrors.NewUnauthorized("MFA challenge is invalid or expired")
		}
		return LoginResponse{}, "", time.Time{}, apperrors.NewInternal("Unable to complete MFA login", err)
	}
	user, err := s.repository.GetUserByID(ctx, userID)
	if err != nil {
		return LoginResponse{}, "", time.Time{}, apperrors.NewUnauthorized("MFA challenge is invalid or expired")
	}
	completedAt := time.Now().UTC()
	method := authnz.AuthenticationMethodTOTP
	if recovery {
		method = authnz.AuthenticationMethodRecoveryCode
	}
	response, token, expiresAt, err := s.createSession(ctx, user, userAgent, ipAddress, method, authnz.AssuranceLevelTwo, &completedAt)
	if err != nil {
		return LoginResponse{}, "", time.Time{}, err
	}
	audit.RecordIdentity(ctx, user.ID, audit.Event{Action: "identity.mfa_login_completed", ResourceType: "user", ResourceID: user.ID.String(), Metadata: map[string]any{"method": method}})
	return LoginResponse{User: &response.User}, token, expiresAt, nil
}

func (s *Service) VerifyEmail(ctx context.Context, req VerifyEmailRequest) error {
	email, token, err := validateEmailToken(req.Email, req.Token)
	if err != nil {
		return err
	}

	user, err := s.repository.GetUserByEmail(ctx, email)
	if err == nil && user.EmailVerified {
		return apperrors.NewBadRequest("Email is already verified")
	}

	identifier := emailVerificationIdentifier(email)
	tokenHash := authnz.HashSessionToken(token)
	user, err = s.repository.VerifyEmailWithToken(ctx, email, identifier, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.NewBadRequest("Email verification token is invalid or expired")
		}
		return apperrors.NewInternal("Unable to verify email", err)
	}
	audit.RecordIdentity(ctx, user.ID, audit.Event{Action: "identity.email_verified", ResourceType: "user", ResourceID: user.ID.String()})
	return nil
}

func (s *Service) ResendEmail(ctx context.Context, req ResendEmailRequest) error {
	email, err := validateEmail(req.Email)
	if err != nil {
		return err
	}
	user, err := s.repository.GetUserByEmail(ctx, email)
	if err != nil {
		return nil
	}
	if user.EmailVerified {
		return nil
	}
	return s.issueEmailVerificationToken(ctx, user.Email, user.Name)
}

func (s *Service) ForgotPassword(ctx context.Context, req ForgotPasswordRequest) error {
	email, err := validateEmail(req.Email)
	if err != nil {
		return err
	}
	user, err := s.repository.GetUserByEmail(ctx, email)
	if err != nil {
		return nil
	}
	return s.issuePasswordResetToken(ctx, user.Email, user.Name)
}

func (s *Service) ResetPassword(ctx context.Context, req ResetPasswordRequest) error {
	email, token, password, err := validateResetPasswordRequest(req)
	if err != nil {
		return err
	}

	identifier := passwordResetIdentifier(email)
	tokenHash := authnz.HashSessionToken(token)
	passwordHash, err := authnz.HashPassword(password)
	if err != nil {
		return apperrors.NewInternal("Unable to hash password", err)
	}
	user, err := s.repository.ResetPasswordWithToken(ctx, email, identifier, tokenHash, passwordHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.NewBadRequest("Password reset token is invalid or expired")
		}
		return apperrors.NewInternal("Unable to reset password", err)
	}
	audit.RecordIdentity(ctx, user.ID, audit.Event{Action: "identity.password_reset", ResourceType: "user", ResourceID: user.ID.String()})
	if notifier, ok := s.notifier.(interface {
		SendPasswordChanged(context.Context, notifications.SendPasswordChangedInput) error
	}); ok {
		if err := notifier.SendPasswordChanged(ctx, notifications.SendPasswordChangedInput{ToEmail: user.Email, Name: user.Name}); err != nil {
			slog.Warn("failed to send password changed notification", "error", err, "user_id", user.ID)
		}
	}
	return nil
}

func (s *Service) Logout(ctx context.Context) error {
	principal, ok := authnz.PrincipalFromContext(ctx)
	if !ok {
		return apperrors.NewUnauthorized("Authentication is required")
	}
	if err := s.sessions.Revoke(ctx, principal.UserID, principal.SessionID); err != nil {
		return apperrors.NewInternal("Unable to revoke session", err)
	}
	return nil
}

func (s *Service) createSession(
	ctx context.Context,
	user UserRecord,
	userAgent *string,
	ipAddress *string,
	method authnz.AuthenticationMethod,
	assurance authnz.AssuranceLevel,
	mfaCompletedAt *time.Time,
) (AuthResponse, string, time.Time, error) {
	knownFingerprint, fingerprintErr := s.sessions.HasKnownFingerprint(ctx, user.ID, userAgent, ipAddress)
	if fingerprintErr != nil {
		slog.Warn("failed to evaluate session fingerprint", "error", fingerprintErr, "user_id", user.ID)
	}
	token, err := authnz.NewSessionToken()
	if err != nil {
		return AuthResponse{}, "", time.Time{}, apperrors.NewInternal(
			"Unable to create session token",
			err,
		)
	}
	authenticatedAt := time.Now().UTC()
	expiresAt := authenticatedAt.Add(sessionTTL)
	if _, err := s.sessions.Create(
		ctx,
		user.ID,
		authnz.HashSessionToken(token),
		userAgent,
		ipAddress,
		expiresAt,
		session.Authentication{
			CredentialVersion: user.CredentialVersion,
			Method:            method,
			Assurance:         assurance,
			AuthenticatedAt:   authenticatedAt,
			MFACompletedAt:    mfaCompletedAt,
		},
	); err != nil {
		return AuthResponse{}, "", time.Time{}, apperrors.NewInternal(
			"Unable to create session",
			err,
		)
	}
	if fingerprintErr == nil && !knownFingerprint {
		s.notifyNewLogin(ctx, user, userAgent, ipAddress, method)
	}
	return AuthResponse{User: authenticatedUserFromRecord(user)}, token, expiresAt, nil
}

func (s *Service) notifyNewLogin(ctx context.Context, user UserRecord, userAgent, ipAddress *string, method authnz.AuthenticationMethod) {
	notifier, ok := s.notifier.(interface {
		SendNewLogin(context.Context, notifications.SendNewLoginInput) error
	})
	if !ok {
		return
	}
	input := notifications.SendNewLoginInput{ToEmail: user.Email, Name: user.Name, Method: string(method), UserAgent: optionalString(userAgent), IPAddress: optionalString(ipAddress)}
	if err := notifier.SendNewLogin(ctx, input); err != nil {
		slog.Warn("failed to send new login notification", "error", err, "user_id", user.ID)
	}
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *Service) issueEmailVerificationToken(ctx context.Context, email string, name string) error {
	return s.issueVerificationToken(
		ctx,
		emailVerificationIdentifier(email),
		emailVerificationTokenTTL,
		func(token string, expiresAt time.Time) error {
			return s.notifier.SendEmailVerification(ctx, notifications.SendEmailVerificationInput{
				ToEmail: email,
				Name:    name,
				Token:   token,
			})
		},
	)
}

func (s *Service) issuePasswordResetToken(ctx context.Context, email string, name string) error {
	return s.issueVerificationToken(
		ctx,
		passwordResetIdentifier(email),
		passwordResetTokenTTL,
		func(token string, expiresAt time.Time) error {
			return s.notifier.SendPasswordReset(ctx, notifications.SendPasswordResetInput{
				ToEmail: email,
				Name:    name,
				Token:   token,
			})
		},
	)
}

func (s *Service) issueVerificationToken(
	ctx context.Context,
	identifier string,
	ttl time.Duration,
	send func(token string, expiresAt time.Time) error,
) error {
	token, err := authnz.NewSessionToken()
	if err != nil {
		return apperrors.NewInternal("Unable to create verification token", err)
	}
	expiresAt := time.Now().UTC().Add(ttl)
	if err := s.repository.CreateVerificationToken(
		ctx,
		identifier,
		authnz.HashSessionToken(token),
		expiresAt,
	); err != nil {
		return apperrors.NewInternal("Unable to store verification token", err)
	}
	if s.notifier != nil {
		if err := send(token, expiresAt); err != nil {
			return apperrors.NewInternal("Unable to deliver verification token", err)
		}
	}
	return nil
}

func authenticatedUserFromRecord(user UserRecord) AuthenticatedUser {
	return AuthenticatedUser{
		ID:            user.ID.String(),
		Email:         user.Email,
		EmailVerified: user.EmailVerified,
		Name:          user.Name,
		CreatedAt:     user.CreatedAt,
		UpdatedAt:     user.UpdatedAt,
	}
}
