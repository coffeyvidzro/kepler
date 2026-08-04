package authnz

import "time"

type AuthenticationMethod string

const (
	AuthenticationMethodPassword     AuthenticationMethod = "password"
	AuthenticationMethodTOTP         AuthenticationMethod = "totp"
	AuthenticationMethodRecoveryCode AuthenticationMethod = "recovery_code"
)

type AssuranceLevel string

const (
	AssuranceLevelOne   AssuranceLevel = "aal1"
	AssuranceLevelTwo   AssuranceLevel = "aal2"
	AssuranceLevelThree AssuranceLevel = "aal3"
)

func (level AssuranceLevel) Meets(required AssuranceLevel) bool {
	requiredRank := assuranceRank(required)
	return requiredRank > 0 && assuranceRank(level) >= requiredRank
}

func (p Principal) RecentlyAuthenticated(required AssuranceLevel, maxAge time.Duration, now time.Time) bool {
	age := now.Sub(p.AuthenticatedAt)
	return p.AssuranceLevel.Meets(required) && !p.AuthenticatedAt.IsZero() && age >= 0 && age <= maxAge
}

func assuranceRank(level AssuranceLevel) int {
	switch level {
	case AssuranceLevelOne:
		return 1
	case AssuranceLevelTwo:
		return 2
	case AssuranceLevelThree:
		return 3
	default:
		return 0
	}
}
