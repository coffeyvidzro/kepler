package billing

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

func (r *Repository) ChargeSMS(
	ctx context.Context,
	tx pgx.Tx,
	input SMSChargeInput,
) (Charge, error) {
	if err := lockTeamBilling(ctx, tx, input.TeamID); err != nil {
		return Charge{}, err
	}
	row, err := r.queries.WithTx(tx).AuthorizeSMSCharge(ctx, dbsqlc.AuthorizeSMSChargeParams{
		TeamID: input.TeamID, ReferenceID: input.MessageID.String(),
		DestinationCountry: input.destinationCountry, Provider: input.provider,
		RouteType: input.routeType, Quantity: int64(input.Segments),
	})
	if err != nil {
		return Charge{}, fmt.Errorf("charge SMS usage: %w", err)
	}
	return Charge{
		Outcome: Outcome(row.Outcome), MarketCode: row.MarketCode, Currency: row.Currency,
		Tier: row.Tier, Product: Product(row.Product), UnitCostUnits: row.UnitCostUnits,
		Quantity: row.Quantity, AmountUnits: row.AmountUnits, RemainingBalance: row.BalanceUnits,
		CoveredByAllowance: row.CoveredByAllowance, RemainingAllowance: row.RemainingAllowance,
	}, nil
}

func (r *Repository) ChargeEmail(
	ctx context.Context,
	tx pgx.Tx,
	input EmailChargeInput,
) (Charge, error) {
	if err := lockTeamBilling(ctx, tx, input.TeamID); err != nil {
		return Charge{}, err
	}
	charge, err := chargeEmailUsage(ctx, tx, input)
	if err != nil {
		return Charge{}, fmt.Errorf("charge email usage: %w", err)
	}
	return charge, nil
}

func lockTeamBilling(ctx context.Context, tx pgx.Tx, teamID uuid.UUID) error {
	if tx == nil {
		return errors.New("billing transaction is required")
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`, teamID); err != nil {
		return fmt.Errorf("lock team billing: %w", err)
	}
	return nil
}
