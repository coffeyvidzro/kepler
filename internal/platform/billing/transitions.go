package billing

import "fmt"

func validateSMSChargeResult(result Charge, _ string) error {
	if result.Outcome != OutcomeApplied &&
		result.Outcome != OutcomeAlreadyApplied &&
		result.Outcome != OutcomeAllowanceApplied {
		return outcomeError(result.Outcome)
	}
	if result.Product != ProductSMS {
		return fmt.Errorf("sms billing charge product resolution mismatch: %s", result.Product)
	}
	return nil
}

func validateEmailChargeResult(result Charge, recipientCount int64) error {
	if result.Outcome != OutcomeApplied &&
		result.Outcome != OutcomeAlreadyApplied &&
		result.Outcome != OutcomeAllowanceApplied {
		return outcomeError(result.Outcome)
	}
	if result.Product != ProductEmail {
		return fmt.Errorf("email billing charge product resolution mismatch: %s", result.Product)
	}
	if result.Quantity != recipientCount {
		return fmt.Errorf(
			"email billing charge quantity mismatch: got %d, want %d",
			result.Quantity,
			recipientCount,
		)
	}
	return nil
}

func outcomeError(outcome Outcome) error {
	switch outcome {
	case OutcomeTeamNotFound:
		return ErrTeamNotFound
	case OutcomeTeamInactive:
		return ErrTeamInactive
	case OutcomeUnsupportedMarket:
		return ErrUnsupportedMarket
	case OutcomeWalletNotFound:
		return ErrWalletNotFound
	case OutcomeRateNotFound:
		return ErrRateNotFound
	case OutcomeCurrencyMismatch:
		return ErrCurrencyMismatch
	case OutcomeInsufficientBalance:
		return ErrInsufficientBalance
	case OutcomeAmountOverflow:
		return ErrAmountOverflow
	default:
		return fmt.Errorf("unknown billing charge outcome: %s", outcome)
	}
}
