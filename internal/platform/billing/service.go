package billing

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type authorizationRepository interface {
	AuthorizeSMS(context.Context, pgx.Tx, SMSAuthorizationInput) (Authorization, error)
	AuthorizeEmail(context.Context, pgx.Tx, EmailAuthorizationInput) (Authorization, error)
}

func (s *Service) AuthorizeEmail(
	ctx context.Context,
	tx pgx.Tx,
	input EmailAuthorizationInput,
) (Authorization, error) {
	if err := validateEmailAuthorization(input); err != nil {
		return Authorization{}, err
	}
	result, err := s.repository.AuthorizeEmail(ctx, tx, input)
	if err != nil {
		return Authorization{}, err
	}
	if err := validateEmailAuthorizationResult(result); err != nil {
		return Authorization{}, fmt.Errorf("authorize email billing: %w", err)
	}
	return result, nil
}

type Service struct {
	repository authorizationRepository
}

func NewService(repository authorizationRepository) *Service {
	return &Service{repository: repository}
}

func (s *Service) AuthorizeSMS(
	ctx context.Context,
	tx pgx.Tx,
	input SMSAuthorizationInput,
) (Authorization, error) {
	input, err := validateSMSAuthorization(input)
	if err != nil {
		return Authorization{}, err
	}
	result, err := s.repository.AuthorizeSMS(ctx, tx, input)
	if err != nil {
		return Authorization{}, err
	}
	if err := validateAuthorization(result, input.destinationCountry); err != nil {
		return Authorization{}, fmt.Errorf("authorize SMS billing: %w", err)
	}
	return result, nil
}
