package mfa

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
)

type fakeStore struct {
	credential Credential
	confirmed  bool
	step       int64
	hashes     []string
	loginUser  uuid.UUID
	tokenHash  string
	loginStep  int64
	loginErr   error
}

func (f *fakeStore) PutUnverified(_ context.Context, _ uuid.UUID, ciphertext []byte) error {
	f.credential = Credential{SecretCiphertext: ciphertext}
	return nil
}
func (f *fakeStore) GetCredential(context.Context, uuid.UUID) (Credential, error) {
	return f.credential, nil
}
func (f *fakeStore) Confirm(_ context.Context, _ uuid.UUID, _ string, step int64, hashes []string) error {
	f.confirmed, f.step, f.hashes = true, step, hashes
	return nil
}
func (f *fakeStore) Verify(context.Context, uuid.UUID, string, int64) error           { return nil }
func (f *fakeStore) UseRecoveryCode(context.Context, uuid.UUID, string, string) error { return nil }
func (f *fakeStore) Disable(context.Context, uuid.UUID, string) error                 { return nil }
func (f *fakeStore) Enabled(context.Context, uuid.UUID) (bool, error) {
	return f.credential.VerifiedAt != nil, nil
}

func (f *fakeStore) CreateLoginChallenge(_ context.Context, tokenHash string, userID uuid.UUID, _ int64, _ time.Time) error {
	f.tokenHash, f.loginUser = tokenHash, userID
	return nil
}
func (f *fakeStore) GetLoginChallenge(context.Context, string) (uuid.UUID, Credential, error) {
	if f.loginErr != nil {
		return uuid.Nil, Credential{}, f.loginErr
	}
	return f.loginUser, f.credential, nil
}
func (f *fakeStore) ConsumeLoginTOTP(_ context.Context, _ string, _ uuid.UUID, step int64) error {
	f.loginStep = step
	return nil
}
func (f *fakeStore) ConsumeLoginRecoveryCode(context.Context, string, uuid.UUID, string) error {
	return nil
}

func TestBeginAndCompleteLoginTOTP(t *testing.T) {
	key := make([]byte, 32)
	cipher, err := authnz.NewSecretCipherKeyring([]string{"test:" + base64.StdEncoding.EncodeToString(key)})
	if err != nil {
		t.Fatal(err)
	}
	secret, err := authnz.NewTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := cipher.Encrypt([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	userID := uuid.New()
	repository := &fakeStore{loginUser: userID, credential: Credential{SecretCiphertext: ciphertext}}
	service := NewService(repository, cipher, "Dugble")
	now := time.Unix(1_700_000_000, 0).UTC()
	service.now = func() time.Time { return now }
	challenge, err := service.BeginLogin(context.Background(), userID, 1)
	if err != nil || challenge == "" || repository.tokenHash == "" {
		t.Fatalf("BeginLogin() = %q, %v", challenge, err)
	}
	got, err := service.CompleteLoginTOTP(context.Background(), challenge, totpAt(secret, now))
	if err != nil || got != userID || repository.loginStep == 0 {
		t.Fatalf("CompleteLoginTOTP() = %s, %v", got, err)
	}
	if _, err := service.CompleteLoginTOTP(context.Background(), "invalid", "000000"); err == nil {
		t.Fatal("accepted malformed login challenge")
	}
}

func TestCompleteLoginPreservesChallengeLookupErrors(t *testing.T) {
	t.Parallel()

	lookupErr := errors.New("database unavailable")
	service := NewService(&fakeStore{loginErr: lookupErr}, nil, "Dugble")

	if _, err := service.CompleteLoginTOTP(context.Background(), loginChallengePrefix+"token", "000000"); !errors.Is(err, lookupErr) {
		t.Fatalf("CompleteLoginTOTP() error = %v, want %v", err, lookupErr)
	}
	if _, err := service.CompleteLoginRecovery(context.Background(), loginChallengePrefix+"token", "recovery"); !errors.Is(err, lookupErr) {
		t.Fatalf("CompleteLoginRecovery() error = %v, want %v", err, lookupErr)
	}
}

func TestEnrollAndConfirm(t *testing.T) {
	key := make([]byte, 32)
	cipher, err := authnz.NewSecretCipherKeyring([]string{"test:" + base64.StdEncoding.EncodeToString(key)})
	if err != nil {
		t.Fatal(err)
	}
	repository := &fakeStore{}
	service := NewService(repository, cipher, "Dugble")
	now := time.Unix(1_700_000_000, 0).UTC()
	service.now = func() time.Time { return now }
	userID := uuid.New()
	ctx := authnz.ContextWithPrincipal(context.Background(), authnz.Principal{UserID: userID, SessionID: "session", Email: "person@example.com"})

	enrollment, err := service.Enroll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if enrollment.Secret == "" || enrollment.URI == "" {
		t.Fatal("expected enrollment secret and URI")
	}
	step, ok := authnz.ValidateTOTP(enrollment.Secret, totpAt(enrollment.Secret, now), now)
	if !ok {
		t.Fatal("test TOTP should be valid")
	}
	response, err := service.Confirm(ctx, totpAt(enrollment.Secret, now))
	if err != nil {
		t.Fatal(err)
	}
	if !repository.confirmed || repository.step != step {
		t.Fatal("confirmation was not persisted")
	}
	if len(response.RecoveryCodes) != recoveryCodeCount || len(repository.hashes) != recoveryCodeCount {
		t.Fatalf("got %d recovery codes", len(response.RecoveryCodes))
	}
	for i, code := range response.RecoveryCodes {
		if authnz.HashRecoveryCode(code) != repository.hashes[i] {
			t.Fatal("recovery code was not hashed before persistence")
		}
	}
}

func totpAt(secret string, now time.Time) string {
	for i := 0; i < 1_000_000; i++ {
		candidate := fmt.Sprintf("%06d", i)
		if _, ok := authnz.ValidateTOTP(secret, candidate, now); ok {
			return candidate
		}
	}
	return ""
}

func (f *fakeStore) RotateSecretCiphertext(context.Context, uuid.UUID, []byte, []byte) error {
	return nil
}
