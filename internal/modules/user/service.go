package user

import (
	"context"
	"log/slog"

	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
	notifications "github.com/coffeyvidzro/dugble/server/internal/platform/systemmail"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

type Service struct {
	repository *Repository
	notifier   SecurityNotifier
}

type SecurityNotifier interface {
	SendPasswordChanged(context.Context, notifications.SendPasswordChangedInput) error
	SendEmailChanged(context.Context, notifications.SendEmailChangedInput) error
	SendAccountDeleted(context.Context, notifications.SendSecurityEventInput) error
}

func NewService(repository *Repository, notifiers ...SecurityNotifier) *Service {
	service := &Service{repository: repository}
	if len(notifiers) > 0 {
		service.notifier = notifiers[0]
	}
	return service
}

func (s *Service) GetMe(ctx context.Context) (User, error) {
	principal, ok := authnz.PrincipalFromContext(ctx)
	if !ok {
		return User{}, apperrors.NewUnauthorized("Authentication is required")
	}
	return s.GetByID(ctx, principal.UserID.String())
}

func (s *Service) GetByID(ctx context.Context, id string) (User, error) {
	id, err := validateID(id)
	if err != nil {
		return User{}, err
	}

	user, err := s.repository.GetByID(ctx, id)
	if err != nil {
		return User{}, apperrors.NewNotFound("User not found")
	}

	return user, nil
}

func (s *Service) UpdateProfile(ctx context.Context, req UpdateProfileRequest) (User, error) {
	principal, ok := authnz.PrincipalFromContext(ctx)
	if !ok {
		return User{}, apperrors.NewUnauthorized("Authentication is required")
	}

	name, err := validateName(req.Name)
	if err != nil {
		return User{}, err
	}

	updated, err := s.repository.UpdateProfile(ctx, principal.UserID.String(), name)
	if err != nil {
		return User{}, apperrors.NewInternal("Unable to update profile", err)
	}

	return updated, nil
}

func (s *Service) UpdateEmail(ctx context.Context, req UpdateEmailRequest) (User, error) {
	principal, ok := authnz.PrincipalFromContext(ctx)
	if !ok {
		return User{}, apperrors.NewUnauthorized("Authentication is required")
	}

	email, err := validateEmail(req.Email)
	if err != nil {
		return User{}, err
	}

	current, err := s.repository.GetByID(ctx, principal.UserID.String())
	if err != nil {
		return User{}, apperrors.NewInternal("Unable to load current email", err)
	}
	updated, err := s.repository.UpdateEmail(ctx, principal.UserID.String(), email)
	if err != nil {
		return User{}, apperrors.NewInternal("Unable to update email", err)
	}

	if s.notifier != nil {
		for _, recipient := range uniqueEmails(current.Email, updated.Email) {
			if err := s.notifier.SendEmailChanged(ctx, notifications.SendEmailChangedInput{ToEmail: recipient, Name: updated.Name, Email: updated.Email}); err != nil {
				slog.Warn("failed to send email changed notification", "error", err, "user_id", updated.ID)
			}
		}
	}
	return updated, nil
}

func (s *Service) UpdatePassword(ctx context.Context, req UpdatePasswordRequest) (User, error) {
	principal, ok := authnz.PrincipalFromContext(ctx)
	if !ok {
		return User{}, apperrors.NewUnauthorized("Authentication is required")
	}

	password, err := validatePassword(req.Password)
	if err != nil {
		return User{}, err
	}

	hash, err := authnz.HashPassword(password)
	if err != nil {
		return User{}, apperrors.NewInternal("Unable to hash password", err)
	}

	updated, err := s.repository.UpdatePassword(ctx, principal.UserID.String(), hash)
	if err != nil {
		return User{}, apperrors.NewInternal("Unable to update password", err)
	}

	if s.notifier != nil {
		if err := s.notifier.SendPasswordChanged(ctx, notifications.SendPasswordChangedInput{ToEmail: updated.Email, Name: updated.Name}); err != nil {
			slog.Warn("failed to send password changed notification", "error", err, "user_id", updated.ID)
		}
	}
	return updated, nil
}

func (s *Service) DeleteMe(ctx context.Context) error {
	principal, ok := authnz.PrincipalFromContext(ctx)
	if !ok {
		return apperrors.NewUnauthorized("Authentication is required")
	}

	current, err := s.repository.GetByID(ctx, principal.UserID.String())
	if err != nil {
		return apperrors.NewInternal("Unable to load user", err)
	}
	if err := s.repository.Delete(ctx, principal.UserID.String()); err != nil {
		return apperrors.NewInternal("Unable to delete user", err)
	}

	if s.notifier != nil {
		if err := s.notifier.SendAccountDeleted(ctx, notifications.SendSecurityEventInput{ToEmail: current.Email, Name: current.Name}); err != nil {
			slog.Warn("failed to send account deleted notification", "error", err, "user_id", current.ID)
		}
	}
	return nil
}
