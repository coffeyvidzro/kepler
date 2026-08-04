package session

import (
	"context"

	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

type Service struct {
	sessions *Repository
}

func NewService(sessions *Repository) *Service {
	return &Service{sessions: sessions}
}

func (s *Service) List(ctx context.Context) ([]Session, error) {
	principal, ok := authnz.PrincipalFromContext(ctx)
	if !ok {
		return nil, apperrors.NewUnauthorized("Authentication is required")
	}
	rows, err := s.sessions.ListByUserID(ctx, principal.UserID)
	if err != nil {
		return nil, apperrors.NewInternal("Unable to list sessions", err)
	}
	out := make([]Session, 0, len(rows))
	for _, row := range rows {
		out = append(out, sessionFromRecord(row))
	}
	return out, nil
}

func (s *Service) Revoke(ctx context.Context, id string) error {
	id, err := validateSessionID(id)
	if err != nil {
		return err
	}
	principal, ok := authnz.PrincipalFromContext(ctx)
	if !ok {
		return apperrors.NewUnauthorized("Authentication is required")
	}
	if err := s.sessions.Revoke(ctx, principal.UserID, id); err != nil {
		return apperrors.NewInternal("Unable to revoke session", err)
	}
	return nil
}

func (s *Service) RevokeOthers(ctx context.Context) error {
	principal, ok := authnz.PrincipalFromContext(ctx)
	if !ok {
		return apperrors.NewUnauthorized("Authentication is required")
	}
	if err := s.sessions.RevokeOthers(ctx, principal.UserID, principal.SessionID); err != nil {
		return apperrors.NewInternal("Unable to revoke other sessions", err)
	}
	return nil
}

func (s *Service) RevokeAll(ctx context.Context) error {
	principal, ok := authnz.PrincipalFromContext(ctx)
	if !ok {
		return apperrors.NewUnauthorized("Authentication is required")
	}
	if err := s.sessions.RevokeAll(ctx, principal.UserID); err != nil {
		return apperrors.NewInternal("Unable to revoke sessions", err)
	}
	return nil
}

func sessionFromRecord(row Record) Session {
	return Session{
		ID:                   row.ID,
		UserAgent:            row.UserAgent,
		IPAddress:            row.IPAddress,
		ExpiresAt:            row.ExpiresAt,
		RevokedAt:            row.RevokedAt,
		CreatedAt:            row.CreatedAt,
		LastSeenAt:           row.LastSeenAt,
		AuthenticationMethod: row.Method,
		AssuranceLevel:       row.Assurance,
		AuthenticatedAt:      row.AuthenticatedAt,
		MFACompletedAt:       row.MFACompletedAt,
	}
}
