package billing

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type fakeAuthorizationRepository struct {
	result Authorization
	err    error
	input  SMSAuthorizationInput
	calls  int
}

func (f *fakeAuthorizationRepository) AuthorizeSMS(
	_ context.Context,
	_ pgx.Tx,
	input SMSAuthorizationInput,
) (Authorization, error) {
	f.calls++
	f.input = input
	return f.result, f.err
}

func (f *fakeAuthorizationRepository) AuthorizeEmail(
	_ context.Context,
	_ pgx.Tx,
	input EmailAuthorizationInput,
) (Authorization, error) {
	f.calls++
	f.input = SMSAuthorizationInput{TeamID: input.TeamID, MessageID: input.MessageID}
	return f.result, f.err
}

func TestAuthorizeSMSNormalizesAndReturnsAppliedCharge(t *testing.T) {
	repository := &fakeAuthorizationRepository{result: Authorization{
		Outcome: OutcomeApplied, MarketCode: "GH", Currency: "GHS", Tier: "growth",
		Product: ProductSMS, UnitCostUnits: 17544, Quantity: 2,
		AmountUnits: 35188, RemainingBalance: 64812,
	}}
	input := SMSAuthorizationInput{
		TeamID: uuid.New(), MessageID: uuid.New(), DestinationNumber: " +2348012345678 ", Segments: 2,
	}
	result, err := NewService(repository).AuthorizeSMS(context.Background(), nil, input)
	if err != nil {
		t.Fatalf("AuthorizeSMS() error = %v", err)
	}
	if repository.input.destinationCountry != "NG" || repository.input.provider != "arkesel" ||
		repository.input.routeType != "standard" || result.Product != ProductSMS || result.AmountUnits != 35188 {
		t.Fatalf("AuthorizeSMS() input/result = %+v/%+v", repository.input, result)
	}
}

func TestAuthorizeSMSAcceptsIdempotentReplay(t *testing.T) {
	repository := &fakeAuthorizationRepository{result: Authorization{
		Outcome: OutcomeAlreadyApplied, MarketCode: "KE", Product: ProductSMS,
	}}
	_, err := NewService(repository).AuthorizeSMS(context.Background(), nil, SMSAuthorizationInput{
		TeamID: uuid.New(), MessageID: uuid.New(), DestinationNumber: "+254712345678", Segments: 1,
	})
	if err != nil {
		t.Fatalf("AuthorizeSMS() replay error = %v", err)
	}
}

func TestAuthorizeSMSMapsAuthorizationOutcomes(t *testing.T) {
	tests := []struct {
		outcome Outcome
		want    error
	}{
		{OutcomeTeamNotFound, ErrTeamNotFound},
		{OutcomeTeamInactive, ErrTeamInactive},
		{OutcomeUnsupportedMarket, ErrUnsupportedMarket},
		{OutcomeWalletNotFound, ErrWalletNotFound},
		{OutcomeRateNotFound, ErrRateNotFound},
		{OutcomeCurrencyMismatch, ErrCurrencyMismatch},
		{OutcomeInsufficientBalance, ErrInsufficientBalance},
		{OutcomeAmountOverflow, ErrAmountOverflow},
	}
	for _, test := range tests {
		repository := &fakeAuthorizationRepository{result: Authorization{Outcome: test.outcome}}
		_, err := NewService(repository).AuthorizeSMS(context.Background(), nil, SMSAuthorizationInput{
			TeamID: uuid.New(), MessageID: uuid.New(), DestinationNumber: "+233241234567", Segments: 1,
		})
		if !errors.Is(err, test.want) {
			t.Fatalf("AuthorizeSMS(%s) error = %v, want %v", test.outcome, err, test.want)
		}
	}
}

func TestAuthorizeSMSRejectsInvalidInputBeforeRepositoryCall(t *testing.T) {
	tests := []SMSAuthorizationInput{
		{MessageID: uuid.New(), DestinationNumber: "+233241234567", Segments: 1},
		{TeamID: uuid.New(), DestinationNumber: "+233241234567", Segments: 1},
		{TeamID: uuid.New(), MessageID: uuid.New(), Segments: 1},
		{TeamID: uuid.New(), MessageID: uuid.New(), DestinationNumber: "+233241234567"},
	}
	for _, input := range tests {
		repository := &fakeAuthorizationRepository{}
		if _, err := NewService(repository).AuthorizeSMS(context.Background(), nil, input); err == nil {
			t.Fatalf("AuthorizeSMS(%+v) accepted invalid input", input)
		}
		if repository.calls != 0 {
			t.Fatalf("AuthorizeSMS(%+v) called repository", input)
		}
	}
}

func TestAuthorizeEmailSupportsAllowanceAndPaidOutcomes(t *testing.T) {
	for _, result := range []Authorization{
		{Outcome: OutcomeAllowanceApplied, Product: ProductEmail, CoveredByAllowance: true, RemainingAllowance: 999},
		{Outcome: OutcomeApplied, Product: ProductEmail, AmountUnits: 936, RemainingBalance: 9064},
		{Outcome: OutcomeAlreadyApplied, Product: ProductEmail},
	} {
		repository := &fakeAuthorizationRepository{result: result}
		got, err := NewService(repository).AuthorizeEmail(context.Background(), nil, EmailAuthorizationInput{
			TeamID: uuid.New(), MessageID: uuid.New(),
		})
		if err != nil {
			t.Fatalf("AuthorizeEmail(%s) error = %v", result.Outcome, err)
		}
		if got.Outcome != result.Outcome || got.Product != ProductEmail {
			t.Fatalf("AuthorizeEmail() = %+v", got)
		}
	}
}

func TestAuthorizeEmailRejectsInvalidInput(t *testing.T) {
	for _, input := range []EmailAuthorizationInput{{MessageID: uuid.New()}, {TeamID: uuid.New()}} {
		repository := &fakeAuthorizationRepository{}
		if _, err := NewService(repository).AuthorizeEmail(context.Background(), nil, input); err == nil {
			t.Fatalf("AuthorizeEmail(%+v) accepted invalid input", input)
		}
		if repository.calls != 0 {
			t.Fatalf("AuthorizeEmail(%+v) called repository", input)
		}
	}
}
