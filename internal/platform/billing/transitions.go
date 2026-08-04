package billing

import "fmt"

func validateAuthorization(result Authorization, _ string) error {
	if result.Outcome != OutcomeApplied &&
		result.Outcome != OutcomeAlreadyApplied &&
		result.Outcome != OutcomeAllowanceApplied {
		return outcomeError(result.Outcome)
	}
	if result.Product != ProductSMS {
		return fmt.Errorf("SMS billing product resolution mismatch: %s", result.Product)
	}
	return nil
}

func validateEmailAuthorizationResult(result Authorization) error {
	if result.Outcome != OutcomeApplied &&
		result.Outcome != OutcomeAlreadyApplied &&
		result.Outcome != OutcomeAllowanceApplied {
		return outcomeError(result.Outcome)
	}
	if result.Product != ProductEmail {
		return fmt.Errorf("email billing product resolution mismatch: %s", result.Product)
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
		return fmt.Errorf("unknown billing authorization outcome: %s", outcome)
	}
}
