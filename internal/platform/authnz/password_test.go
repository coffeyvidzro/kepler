package authnz

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestVerifyPassword(t *testing.T) {
	passwordHash, err := HashPassword("correct-password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	malformedHash := "not-a-bcrypt-hash"

	tests := []struct {
		name     string
		hash     *string
		password string
		want     bool
	}{
		{
			name:     "matching stored password",
			hash:     &passwordHash,
			password: "correct-password",
			want:     true,
		},
		{
			name:     "wrong stored password",
			hash:     &passwordHash,
			password: "wrong-password",
			want:     false,
		},
		{
			name:     "missing password hash",
			hash:     nil,
			password: "kepler-login-timing-dummy-password-v1",
			want:     false,
		},
		{
			name:     "malformed password hash",
			hash:     &malformedHash,
			password: "correct-password",
			want:     false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := VerifyPassword(test.hash, test.password); got != test.want {
				t.Fatalf("VerifyPassword() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestDummyPasswordHashUsesConfiguredCost(t *testing.T) {
	cost, err := bcrypt.Cost([]byte(dummyPasswordHash))
	if err != nil {
		t.Fatalf("parse dummy password hash: %v", err)
	}
	if cost != passwordHashCost {
		t.Fatalf("dummy password hash cost = %d, want %d", cost, passwordHashCost)
	}
}
