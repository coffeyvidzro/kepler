package wallet

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
)

type Repository struct {
	queries *dbsqlc.Queries
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{queries: dbsqlc.New(db)}
}

func (r *Repository) Get(ctx context.Context, teamID uuid.UUID) (Wallet, error) {
	row, err := r.queries.GetTeamWallet(ctx, dbsqlc.GetTeamWalletParams{TeamID: teamID})
	if err != nil {
		return Wallet{}, fmt.Errorf("get team wallet: %w", err)
	}
	return walletFromSQLC(row), nil
}

func (r *Repository) ListLedger(
	ctx context.Context,
	teamID uuid.UUID,
	limit int32,
	offset int32,
) ([]LedgerEntry, error) {
	rows, err := r.queries.ListWalletLedger(ctx, dbsqlc.ListWalletLedgerParams{
		TeamID: teamID, OffsetCount: offset, LimitCount: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list wallet ledger: %w", err)
	}
	entries := make([]LedgerEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, ledgerEntryFromSQLC(row))
	}
	return entries, nil
}

func (r *Repository) Credit(
	ctx context.Context,
	teamID uuid.UUID,
	amountUnits int64,
	referenceID string,
) (Wallet, error) {
	row, err := r.queries.CreditTeamWallet(ctx, dbsqlc.CreditTeamWalletParams{
		TeamID:          teamID,
		AmountUnits:     amountUnits,
		TransactionType: "deposit",
		ReferenceID:     referenceID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return r.Get(ctx, teamID)
		}
		return Wallet{}, fmt.Errorf("credit team wallet: %w", err)
	}
	return Wallet{
		TeamID: row.TeamID.String(), BillingMarket: row.BillingMarket,
		Currency: row.Currency, BalanceUnits: row.BalanceUnits, Tier: row.Tier,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

func walletFromSQLC(row dbsqlc.TeamWallet) Wallet {
	return Wallet{
		TeamID: row.TeamID.String(), BillingMarket: row.BillingMarket,
		Currency: row.Currency, BalanceUnits: row.BalanceUnits, Tier: row.Tier,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func ledgerEntryFromSQLC(row dbsqlc.WalletLedger) LedgerEntry {
	var authorizationID *string
	if row.UsageAuthorizationID != nil {
		value := row.UsageAuthorizationID.String()
		authorizationID = &value
	}
	return LedgerEntry{
		ID: row.ID.String(), TeamID: row.TeamID.String(), UsageAuthorizationID: authorizationID,
		AmountUnits: row.AmountUnits, TransactionType: row.TransactionType,
		ReferenceID: row.ReferenceID, CreatedAt: row.CreatedAt.Time,
	}
}
