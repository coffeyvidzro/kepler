package billing

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type chargeRepositoryStub struct {
	emailInput  EmailChargeInput
	emailResult Charge
	emailErr    error
	emailCalls  int
}

func (stub *chargeRepositoryStub) ChargeEmail(
	_ context.Context,
	_ pgx.Tx,
	input EmailChargeInput,
) (Charge, error) {
	stub.emailCalls++
	stub.emailInput = input
	return stub.emailResult, stub.emailErr
}

func (*chargeRepositoryStub) ChargeSMS(
	context.Context,
	pgx.Tx,
	SMSChargeInput,
) (Charge, error) {
	return Charge{}, errors.New("unexpected SMS charge")
}

func TestChargeEmailPassesRecipientCount(t *testing.T) {
	t.Parallel()

	teamID := uuid.New()
	messageID := uuid.New()
	repository := &chargeRepositoryStub{
		emailResult: Charge{
			Outcome: OutcomeApplied, Product: ProductEmail, Quantity: 4,
		},
	}
	service := NewService(repository)

	result, err := service.ChargeEmail(context.Background(), nil, EmailChargeInput{
		TeamID: teamID, MessageID: messageID, RecipientCount: 4,
	})
	if err != nil {
		t.Fatalf("ChargeEmail() error = %v", err)
	}
	if repository.emailCalls != 1 {
		t.Fatalf("ChargeEmail() repository calls = %d, want 1", repository.emailCalls)
	}
	if repository.emailInput.RecipientCount != 4 {
		t.Fatalf(
			"ChargeEmail() recipient count = %d, want 4",
			repository.emailInput.RecipientCount,
		)
	}
	if result.Quantity != 4 {
		t.Fatalf("ChargeEmail() quantity = %d, want 4", result.Quantity)
	}
}

func TestChargeEmailRejectsInvalidRecipientCount(t *testing.T) {
	t.Parallel()

	repository := &chargeRepositoryStub{}
	service := NewService(repository)

	_, err := service.ChargeEmail(context.Background(), nil, EmailChargeInput{
		TeamID: uuid.New(), MessageID: uuid.New(), RecipientCount: 0,
	})
	if !errors.Is(err, ErrInvalidRecipientCount) {
		t.Fatalf("ChargeEmail() error = %v, want %v", err, ErrInvalidRecipientCount)
	}
	if repository.emailCalls != 0 {
		t.Fatalf("ChargeEmail() repository calls = %d, want 0", repository.emailCalls)
	}
}

func TestChargeEmailRejectsQuantityMismatch(t *testing.T) {
	t.Parallel()

	repository := &chargeRepositoryStub{
		emailResult: Charge{
			Outcome: OutcomeApplied, Product: ProductEmail, Quantity: 1,
		},
	}
	service := NewService(repository)

	_, err := service.ChargeEmail(context.Background(), nil, EmailChargeInput{
		TeamID: uuid.New(), MessageID: uuid.New(), RecipientCount: 3,
	})
	if err == nil {
		t.Fatal("ChargeEmail() error = nil, want quantity mismatch")
	}
}
