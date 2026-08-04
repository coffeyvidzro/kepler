package keyrotation

import (
	"context"
	"fmt"

	db "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Result struct {
	Scanned int
	Rotated int
}
type Service struct {
	q      *db.Queries
	cipher *authnz.SecretCipher
}

func New(pool *pgxpool.Pool, cipher *authnz.SecretCipher) *Service {
	return &Service{db.New(pool), cipher}
}
func (s *Service) Rotate(ctx context.Context) (Result, error) {
	var result Result
	totp, err := s.q.ListTOTPSecretsForRotation(ctx)
	if err != nil {
		return result, fmt.Errorf("list TOTP secrets: %w", err)
	}
	for _, row := range totp {
		result.Scanned++
		_, replacement, rotate, err := s.cipher.DecryptAndRotate(row.SecretCiphertext)
		if err != nil {
			return result, fmt.Errorf("decrypt TOTP secret for %s: %w", row.UserID, err)
		}
		if rotate {
			if err = s.q.RotateTOTPSecretCiphertext(ctx, db.RotateTOTPSecretCiphertextParams{UserID: row.UserID, OldCiphertext: row.SecretCiphertext, NewCiphertext: replacement}); err != nil {
				return result, err
			}
			result.Rotated++
		}
	}
	return result, nil
}
