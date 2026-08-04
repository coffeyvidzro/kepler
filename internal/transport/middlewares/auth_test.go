package middlewares

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/modules/session"
	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
)

type sessionStoreStub struct {
	record session.Record
	err    error
}

func (s sessionStoreStub) GetByTokenHash(context.Context, string) (session.Record, error) {
	return s.record, s.err
}

func (sessionStoreStub) Touch(context.Context, string) error { return nil }

type principalRepositoryStub struct {
	principal authnz.Principal
	err       error
}

func (s principalRepositoryStub) GetPrincipalByUserID(context.Context, string) (authnz.Principal, error) {
	if s.err != nil {
		return authnz.Principal{}, s.err
	}
	return s.principal, nil
}

func TestSessionAuthClassifiesLookupErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "missing session", err: pgx.ErrNoRows, wantStatus: http.StatusUnauthorized},
		{name: "wrapped missing session", err: fmt.Errorf("lookup: %w", pgx.ErrNoRows), wantStatus: http.StatusUnauthorized},
		{name: "database failure", err: errors.New("database unavailable"), wantStatus: http.StatusInternalServerError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodGet, "/sessions", nil)
			request.AddCookie(&http.Cookie{Name: authnz.SessionCookieName, Value: "secret"})
			response := httptest.NewRecorder()
			ctx := echo.New().NewContext(request, response)
			handler := SessionAuth(SessionAuthConfig{
				Sessions: sessionStoreStub{err: test.err},
				Users:    principalRepositoryStub{err: errors.New("unexpected principal lookup")},
			})(func(*echo.Context) error {
				t.Fatal("next handler must not run")
				return nil
			})

			if err := handler(ctx); err != nil {
				t.Fatalf("SessionAuth() error = %v", err)
			}
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
		})
	}
}

func TestSessionAuthRejectsStaleCredentialVersion(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	request := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	request.AddCookie(&http.Cookie{Name: authnz.SessionCookieName, Value: "secret"})
	response := httptest.NewRecorder()
	ctx := echo.New().NewContext(request, response)
	handler := SessionAuth(SessionAuthConfig{
		Sessions: sessionStoreStub{record: session.Record{UserID: userID, ExpiresAt: time.Now().Add(time.Hour), Authentication: session.Authentication{CredentialVersion: 1}}},
		Users:    principalRepositoryStub{principal: authnz.Principal{UserID: userID, CredentialVersion: 2}},
	})(func(*echo.Context) error {
		t.Fatal("next handler must not run for a stale session")
		return nil
	})

	if err := handler(ctx); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestRequireRecentAuthentication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		principal  authnz.Principal
		wantStatus int
		wantNext   bool
	}{
		{name: "recent aal2", principal: authnz.Principal{AssuranceLevel: authnz.AssuranceLevelTwo, AuthenticatedAt: time.Now().Add(-time.Minute)}, wantStatus: http.StatusNoContent, wantNext: true},
		{name: "insufficient assurance", principal: authnz.Principal{AssuranceLevel: authnz.AssuranceLevelOne, AuthenticatedAt: time.Now()}, wantStatus: http.StatusForbidden},
		{name: "stale authentication", principal: authnz.Principal{AssuranceLevel: authnz.AssuranceLevelThree, AuthenticatedAt: time.Now().Add(-time.Hour)}, wantStatus: http.StatusForbidden},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodPost, "/sensitive", nil)
			request = request.WithContext(authnz.ContextWithPrincipal(request.Context(), test.principal))
			response := httptest.NewRecorder()
			ctx := echo.New().NewContext(request, response)
			nextCalled := false
			handler := RequireRecentAuthentication(StepUpConfig{Assurance: authnz.AssuranceLevelTwo, MaxAge: 15 * time.Minute})(func(c *echo.Context) error {
				nextCalled = true
				return c.NoContent(http.StatusNoContent)
			})
			if err := handler(ctx); err != nil {
				t.Fatal(err)
			}
			if response.Code != test.wantStatus || nextCalled != test.wantNext {
				t.Fatalf("status = %d, next = %t; want %d, %t", response.Code, nextCalled, test.wantStatus, test.wantNext)
			}
		})
	}
}
