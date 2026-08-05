package authnz

import "errors"

var (
	ErrEncryptionKeyRequired      = errors.New("at least one encryption key is required")
	ErrInvalidEncryptionKeySpec   = errors.New("invalid encryption key specification")
	ErrInvalidEncryptionKey       = errors.New("invalid encryption key")
	ErrInvalidEncryptedSecret     = errors.New("invalid encrypted secret")
	ErrEncryptionKeyNotConfigured = errors.New("encryption key is not configured")
	ErrSecretDecryptionFailed     = errors.New("unable to decrypt secret with configured keys")
	ErrInvalidTokenSize           = errors.New("token size must be greater than zero")
)
