package billing

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type chargeRepository interface {
	ChargeSMS(context.Context, pgx.Tx, SMSChargeInput) (Charge, error)
	ChargeEmail(context.Context, pgx.Tx, EmailChargeInput) (Charge, error)
}

type Service struct {
	repository chargeRepository
}

func NewService(repository chargeRepository) *Service {
	return &Service{repository: repository}
}

func (s *Service) ChargeEmail(
	ctx context.Context,
	tx pgx.Tx,
	input EmailChargeInput,
) (Charge, error) {
	if err := validateEmailCharge(input); err != nil {
		return Charge{}, err
	}
	result, err := s.repository.ChargeEmail(ctx, tx, input)
	if err != nil {
		return Charge{}, err
	}
	if err := validateEmailChargeResult(result, input.RecipientCount); err != nil {
		return Charge{}, fmt.Errorf("charge email billing: %w", err)
	}
	return result, nil
}

func (s *Service) ChargeSMS(
	ctx context.Context,
	tx pgx.Tx,
	input SMSChargeInput,
) (Charge, error) {
	input, err := validateSMSCharge(input)
	if err != nil {
		return Charge{}, err
	}
	result, err := s.repository.ChargeSMS(ctx, tx, input)
	if err != nil {
		return Charge{}, err
	}
	if err := validateSMSChargeResult(result, input.destinationCountry); err != nil {
		return Charge{}, fmt.Errorf("charge SMS billing: %w", err)
	}
	return result, nil
}
