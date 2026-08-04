package billing

import "errors"

type Outcome string

const (
	OutcomeApplied             Outcome = "applied"
	OutcomeAlreadyApplied      Outcome = "already_applied"
	OutcomeAllowanceApplied    Outcome = "allowance_applied"
	OutcomeInsufficientBalance Outcome = "insufficient_balance"
	OutcomeTeamNotFound        Outcome = "team_not_found"
	OutcomeTeamInactive        Outcome = "team_inactive"
	OutcomeUnsupportedMarket   Outcome = "unsupported_market"
	OutcomeWalletNotFound      Outcome = "wallet_not_found"
	OutcomeRateNotFound        Outcome = "rate_not_found"
	OutcomeCurrencyMismatch    Outcome = "currency_mismatch"
	OutcomeAmountOverflow      Outcome = "amount_overflow"
)

var (
	ErrTeamNotFound        = errors.New("billing team not found")
	ErrTeamInactive        = errors.New("billing team is not active")
	ErrUnsupportedMarket   = errors.New("billing market is not supported")
	ErrWalletNotFound      = errors.New("team wallet not found")
	ErrRateNotFound        = errors.New("active product rate not found")
	ErrCurrencyMismatch    = errors.New("billing currency does not match team market")
	ErrInsufficientBalance = errors.New("insufficient wallet balance")
	ErrAmountOverflow      = errors.New("billing amount exceeds supported range")
)
