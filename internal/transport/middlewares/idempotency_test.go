package middlewares

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
	"github.com/coffeyvidzro/dugble/server/internal/platform/idempotency"
)

type memoryIdempotencyRepository struct {
	records map[string]idempotency.Record
}

func (r *memoryIdempotencyRepository) recordKey(scope, key string) string { return scope + "\n" + key }

func (r *memoryIdempotencyRepository) CreateProcessing(_ context.Context, record idempotency.Record) (idempotency.Record, error) {
	key := r.recordKey(record.Scope, record.Key)
	if _, exists := r.records[key]; exists {
		return idempotency.Record{}, idempotency.ErrAlreadyExists
	}
	record.Status = idempotency.StatusProcessing
	r.records[key] = record
	return record, nil
}

func (r *memoryIdempotencyRepository) Get(_ context.Context, scope, key string) (idempotency.Record, error) {
	record, exists := r.records[r.recordKey(scope, key)]
	if !exists {
		return idempotency.Record{}, errors.New("record not found")
	}
	return record, nil
}

func (r *memoryIdempotencyRepository) Complete(_ context.Context, scope, key string, status int, body []byte, contentType string, headers []byte) error {
	record := r.records[r.recordKey(scope, key)]
	responseStatus := int32(status)
	record.Status, record.ResponseStatus = idempotency.StatusCompleted, &responseStatus
	record.ResponseBody, record.ResponseContentType, record.ResponseHeaders = append([]byte(nil), body...), &contentType, append([]byte(nil), headers...)
	r.records[r.recordKey(scope, key)] = record
	return nil
}

func (r *memoryIdempotencyRepository) Delete(_ context.Context, scope, key string) error {
	delete(r.records, r.recordKey(scope, key))
	return nil
}

func TestIdempotencyReplaysCompletedSingleSend(t *testing.T) {
	repository := &memoryIdempotencyRepository{records: make(map[string]idempotency.Record)}
	router := echo.New()
	router.Use(Idempotency(IdempotencyConfig{Repository: repository}))
	calls := 0
	router.POST("/sms", func(c *echo.Context) error {
		calls++
		c.Response().Header().Set("Location", "/sms/message-1")
		return c.JSON(http.StatusAccepted, map[string]string{"id": "message-1"})
	})

	send := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/sms", strings.NewReader(`{"to":"+233241234567"}`))
		request.Header.Set(echo.HeaderAuthorization, "Bearer team-token")
		request.Header.Set(idempotency.Header, "send-1")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}

	first, replay := send(), send()
	if calls != 1 {
		t.Fatalf("single-send handler calls = %d, want 1", calls)
	}
	if first.Code != http.StatusAccepted || replay.Code != first.Code || replay.Body.String() != first.Body.String() {
		t.Fatalf("replayed response = (%d, %q), want (%d, %q)", replay.Code, replay.Body.String(), first.Code, first.Body.String())
	}
	if replay.Header().Get("Location") != "/sms/message-1" {
		t.Fatalf("replayed Location = %q", replay.Header().Get("Location"))
	}
}

func TestCanonicalTeamIDNormalizesHeader(t *testing.T) {
	t.Parallel()
	teamID := uuid.New()
	request := httptest.NewRequest(http.MethodPost, "/emails", nil)
	request.Header.Set("X-Team-ID", strings.ToUpper(teamID.String()))

	got, ok, err := canonicalTeamID(request)
	if err != nil || !ok || got != teamID.String() {
		t.Fatalf("canonicalTeamID() = %q, %v, %v; want %q, true, nil", got, ok, err, teamID)
	}
}

func TestCanonicalTeamIDRejectsInvalidHeader(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest(http.MethodPost, "/emails", nil)
	request.Header.Set("X-Team-ID", "not-a-team")
	if _, _, err := canonicalTeamID(request); err == nil {
		t.Fatal("expected invalid team header to be rejected")
	}
}

func TestRequestIdempotencyScopeUsesSessionCookie(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodPost, "/sms/messages", nil)
	request.AddCookie(&http.Cookie{Name: authnz.SessionCookieName, Value: "session-secret"})

	scope, ok := requestIdempotencyScope(request)
	if !ok {
		t.Fatal("expected session cookie scope")
	}
	if !strings.HasPrefix(scope, "session:") {
		t.Fatalf("scope = %q, want session-prefixed scope", scope)
	}
	if strings.Contains(scope, "session-secret") {
		t.Fatal("scope should not contain the raw session credential")
	}
}

func TestRequestIdempotencyScopeUsesBearerToken(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodPost, "/sms/messages", nil)
	request.Header.Set(echo.HeaderAuthorization, "Bearer dugble_token_secret")

	scope, ok := requestIdempotencyScope(request)
	if !ok {
		t.Fatal("expected bearer token scope")
	}
	if !strings.HasPrefix(scope, "bearer:") {
		t.Fatalf("scope = %q, want bearer-prefixed scope", scope)
	}
	if strings.Contains(scope, "dugble_token_secret") {
		t.Fatal("scope should not contain the raw bearer credential")
	}
}

func TestHashRequestIncludesQueryAndBody(t *testing.T) {
	t.Parallel()

	base := hashRequest(http.MethodPost, "/sms/messages", "team_id=123", []byte(`{"body":"one"}`))
	if base == hashRequest(http.MethodPost, "/sms/messages", "team_id=456", []byte(`{"body":"one"}`)) {
		t.Fatal("expected different query strings to produce different hashes")
	}
	if base == hashRequest(http.MethodPost, "/sms/messages", "team_id=123", []byte(`{"body":"two"}`)) {
		t.Fatal("expected different bodies to produce different hashes")
	}
}

func TestSMSMutationIdempotencyHashesIncludePathAndSchedule(t *testing.T) {
	update := hashRequest(http.MethodPatch, "/sms/message-id", "", []byte(`{"scheduled_at":"2026-08-05T13:00:00Z"}`))
	if update == hashRequest(http.MethodPatch, "/sms/other-id", "", []byte(`{"scheduled_at":"2026-08-05T13:00:00Z"}`)) {
		t.Fatal("different SMS IDs must produce different idempotency hashes")
	}
	if update == hashRequest(http.MethodPatch, "/sms/message-id", "", []byte(`{"scheduled_at":"2026-08-05T14:00:00Z"}`)) {
		t.Fatal("different SMS schedules must produce different idempotency hashes")
	}
	if update == hashRequest(http.MethodPost, "/sms/message-id/cancel", "", nil) {
		t.Fatal("SMS update and cancellation must produce different idempotency hashes")
	}
}

func TestReadAndRestoreBodyAllowsDownstreamRead(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodPost, "/sms/messages", strings.NewReader(`{"message":"hello"}`))
	body, err := readAndRestoreBody(request)
	if err != nil {
		t.Fatalf("readAndRestoreBody returned error: %v", err)
	}
	if string(body) != `{"message":"hello"}` {
		t.Fatalf("body = %q, want original body", body)
	}

	restored, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatalf("read restored body: %v", err)
	}
	if string(restored) != string(body) {
		t.Fatalf("restored body = %q, want %q", restored, body)
	}
}

func TestEncodeResponseHeadersFiltersNonReplayableHeaders(t *testing.T) {
	t.Parallel()

	headers := http.Header{}
	headers.Set(echo.HeaderContentType, "application/json")
	headers.Set(echo.HeaderSetCookie, "dugble_session=secret")
	headers.Set(echo.HeaderXRequestID, "request-id")

	encoded, err := encodeResponseHeaders(headers)
	if err != nil {
		t.Fatalf("encodeResponseHeaders returned error: %v", err)
	}

	restored := http.Header{}
	if err := restoreResponseHeaders(restored, encoded); err != nil {
		t.Fatalf("restoreResponseHeaders returned error: %v", err)
	}
	if restored.Get(echo.HeaderContentType) != "application/json" {
		t.Fatalf("content type = %q, want application/json", restored.Get(echo.HeaderContentType))
	}
	if restored.Get(echo.HeaderSetCookie) != "" {
		t.Fatal("expected Set-Cookie to be filtered from replayable headers")
	}
	if restored.Get(echo.HeaderXRequestID) != "" {
		t.Fatal("expected X-Request-Id to be filtered from replayable headers")
	}
}
