package wallet

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

type fakeStore struct {
	wallet       Wallet
	entries      []LedgerEntry
	err          error
	teamID       uuid.UUID
	limit        int32
	offset       int32
	amountUnits  int64
	referenceID  string
	creditCalled bool
}

func (f *fakeStore) Get(_ context.Context, teamID uuid.UUID) (Wallet, error) {
	f.teamID = teamID
	return f.wallet, f.err
}

func (f *fakeStore) ListLedger(
	_ context.Context,
	teamID uuid.UUID,
	limit int32,
	offset int32,
) ([]LedgerEntry, error) {
	f.teamID, f.limit, f.offset = teamID, limit, offset
	return f.entries, f.err
}

func (f *fakeStore) Credit(
	_ context.Context,
	teamID uuid.UUID,
	amountUnits int64,
	referenceID string,
) (Wallet, error) {
	f.teamID, f.amountUnits, f.referenceID, f.creditCalled = teamID, amountUnits, referenceID, true
	return f.wallet, f.err
}

func TestGetUsesAuthorizedTeamScope(t *testing.T) {
	teamID := uuid.New()
	repository := &fakeStore{wallet: Wallet{TeamID: teamID.String(), Currency: "GHS"}}
	wallet, err := NewService(repository).Get(walletAccessContext(teamID))
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if wallet.TeamID != teamID.String() || repository.teamID != teamID {
		t.Fatalf("Get() wallet/team = %q/%s", wallet.TeamID, repository.teamID)
	}
}

func TestGetMapsMissingWalletToNotFound(t *testing.T) {
	_, err := NewService(&fakeStore{err: pgx.ErrNoRows}).Get(walletAccessContext(uuid.New()))
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "NOT_FOUND" {
		t.Fatalf("Get() error = %v, want NOT_FOUND", err)
	}
}

func TestListLedgerUsesDefaultPagination(t *testing.T) {
	teamID := uuid.New()
	repository := &fakeStore{entries: []LedgerEntry{{ID: uuid.NewString()}}}
	page, err := NewService(repository).ListLedger(walletAccessContext(teamID), 0, 0)
	if err != nil {
		t.Fatalf("ListLedger() error = %v", err)
	}
	if repository.teamID != teamID || repository.limit != defaultLedgerLimit || repository.offset != 0 {
		t.Fatalf("ListLedger() args = %s, %d, %d", repository.teamID, repository.limit, repository.offset)
	}
	if len(page.Entries) != 1 || page.Limit != defaultLedgerLimit {
		t.Fatalf("ListLedger() page = %+v", page)
	}
}

func TestListLedgerRejectsInvalidPagination(t *testing.T) {
	service := NewService(&fakeStore{})
	for _, input := range [][2]int32{{-1, 0}, {101, 0}, {10, -1}} {
		if _, err := service.ListLedger(walletAccessContext(uuid.New()), input[0], input[1]); err == nil {
			t.Fatalf("ListLedger(%d, %d) accepted invalid pagination", input[0], input[1])
		}
	}
}

func TestCreditValidatesAndNormalizesPaymentInput(t *testing.T) {
	teamID := uuid.New()
	repository := &fakeStore{wallet: Wallet{TeamID: teamID.String(), BalanceUnits: 500}}
	wallet, err := NewService(repository).Credit(context.Background(), CreditInput{
		TeamID: " " + teamID.String() + " ", AmountUnits: 500, ReferenceID: " payment-123 ",
	})
	if err != nil {
		t.Fatalf("Credit() error = %v", err)
	}
	if wallet.BalanceUnits != 500 || repository.teamID != teamID || repository.amountUnits != 500 || repository.referenceID != "payment-123" {
		t.Fatalf("Credit() result/store = %+v/%+v", wallet, repository)
	}
}

func TestCreditRejectsInvalidInputBeforeRepositoryCall(t *testing.T) {
	for _, input := range []CreditInput{
		{TeamID: "invalid", AmountUnits: 1, ReferenceID: "payment"},
		{TeamID: uuid.NewString(), AmountUnits: 0, ReferenceID: "payment"},
		{TeamID: uuid.NewString(), AmountUnits: 1, ReferenceID: " "},
	} {
		repository := &fakeStore{}
		if _, err := NewService(repository).Credit(context.Background(), input); err == nil {
			t.Fatalf("Credit(%+v) accepted invalid input", input)
		}
		if repository.creditCalled {
			t.Fatalf("Credit(%+v) called repository", input)
		}
	}
}

func TestWalletReadsRequirePermission(t *testing.T) {
	service := NewService(&fakeStore{})
	if _, err := service.Get(context.Background()); err == nil {
		t.Fatal("Get() allowed missing tenant context")
	}
	if _, err := service.ListLedger(context.Background(), 10, 0); err == nil {
		t.Fatal("ListLedger() allowed missing tenant context")
	}
}

func walletAccessContext(teamID uuid.UUID) context.Context {
	return tenant.ContextWithAccess(context.Background(), tenant.AccessContext{
		Actor: tenant.Actor{Type: tenant.ActorTypeUser, UserID: uuid.New()},
		Scope: tenant.Scope{TeamID: teamID, Role: tenant.RoleOwner, Status: tenant.StatusActive},
	})
}
