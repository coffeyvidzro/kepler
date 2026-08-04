package mfa

import "testing"

func TestMFAValidation(t *testing.T) {
	if got, err := validateRecoveryCode(" recovery "); err != nil || got != "recovery" {
		t.Fatalf("validateRecoveryCode() = %q, %v", got, err)
	}
	if _, err := validateRecoveryCode(" "); err == nil {
		t.Fatal("validateRecoveryCode() accepted an empty code")
	}
	if hash, err := validateLoginChallengeToken(loginChallengePrefix + "token"); err != nil || hash == "" {
		t.Fatalf("validateLoginChallengeToken() = %q, %v", hash, err)
	}
	if _, err := validateLoginChallengeToken("invalid"); err == nil {
		t.Fatal("validateLoginChallengeToken() accepted an invalid prefix")
	}
}
