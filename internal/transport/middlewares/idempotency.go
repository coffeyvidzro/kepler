package middlewares

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
	"github.com/coffeyvidzro/dugble/server/internal/platform/idempotency"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

const (
	defaultLockTTL   = 30 * time.Second
	defaultRecordTTL = 24 * time.Hour
)

var nonReplayableResponseHeaders = map[string]struct{}{
	"Connection":          {},
	"Content-Length":      {},
	"Date":                {},
	"Keep-Alive":          {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"Server":              {},
	"Set-Cookie":          {},
	"Te":                  {},
	"Trailer":             {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
	"X-Request-Id":        {},
}

type IdempotencyConfig struct {
	Repository idempotencyRepository
	LockTTL    time.Duration
	RecordTTL  time.Duration
}

type idempotencyRepository interface {
	CreateProcessing(context.Context, idempotency.Record) (idempotency.Record, error)
	Get(context.Context, string, string) (idempotency.Record, error)
	Complete(context.Context, string, string, int, []byte, string, []byte) error
	Delete(context.Context, string, string) error
}

func Idempotency(config IdempotencyConfig) echo.MiddlewareFunc {
	lockTTL := config.LockTTL
	if lockTTL <= 0 {
		lockTTL = defaultLockTTL
	}

	recordTTL := config.RecordTTL
	if recordTTL <= 0 {
		recordTTL = defaultRecordTTL
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if config.Repository == nil || !isIdempotencyCandidate(c.Request().Method) {
				return next(c)
			}

			path := c.Path()
			if path == "" {
				path = c.Request().URL.Path
			}

			key := strings.TrimSpace(c.Request().Header.Get(idempotency.Header))
			if key == "" || isCookieSessionRoute(path) {
				return next(c)
			}
			if _, err := idempotency.ValidateKey(key); errors.Is(err, idempotency.ErrKeyTooLong) {
				return httputil.Error(c, apperrors.NewBadRequest("Idempotency-Key must be at most 256 characters"))
			}

			scope, ok := requestIdempotencyScope(c.Request())
			if !ok {
				return next(c)
			}
			teamID, hasTeam, err := canonicalTeamID(c.Request())
			if err != nil {
				return httputil.Error(c, apperrors.NewBadRequest("X-Team-ID must be a valid UUID"))
			}
			if hasTeam {
				scope += ":team:" + teamID
			}

			body, err := readAndRestoreBody(c.Request())
			if err != nil {
				return httputil.Error(c, apperrors.NewBadRequest("Unable to read request body"))
			}

			ctx := c.Request().Context()
			requestHash := hashRequest(
				c.Request().Method,
				path,
				c.Request().URL.Query().Encode(),
				body,
			)
			now := time.Now().UTC()

			_, err = config.Repository.CreateProcessing(ctx, idempotency.Record{
				Scope:       scope,
				Key:         key,
				Method:      c.Request().Method,
				Path:        path,
				RequestHash: requestHash,
				LockedUntil: now.Add(lockTTL),
				ExpiresAt:   now.Add(recordTTL),
			})
			if err != nil {
				if errors.Is(err, idempotency.ErrAlreadyExists) {
					return replayOrReject(ctx, c, config.Repository, scope, key, requestHash, now)
				}

				return httputil.Error(
					c,
					apperrors.NewInternal("Unable to reserve idempotency key", err),
				)
			}

			recorder := newResponseRecorder(c.Response())
			c.SetResponse(recorder)

			if err := next(c); err != nil {
				_ = config.Repository.Delete(ctx, scope, key)
				return err
			}

			status := recorder.status
			if status == 0 {
				status = http.StatusOK
			}

			responseHeaders, err := encodeResponseHeaders(recorder.Header())
			if err != nil {
				_ = config.Repository.Delete(ctx, scope, key)
				return httputil.Error(
					c,
					apperrors.NewInternal("Unable to store idempotent response headers", err),
				)
			}

			if err := config.Repository.Complete(
				ctx,
				scope,
				key,
				status,
				recorder.body.Bytes(),
				recorder.Header().Get(echo.HeaderContentType),
				responseHeaders,
			); err != nil {
				return httputil.Error(
					c,
					apperrors.NewInternal("Unable to complete idempotency key", err),
				)
			}

			return recorder.Flush()
		}
	}
}

func canonicalTeamID(request *http.Request) (string, bool, error) {
	value := strings.TrimSpace(request.Header.Get("X-Team-ID"))
	if value == "" {
		return "", false, nil
	}
	teamID, err := uuid.Parse(value)
	if err != nil {
		return "", false, err
	}
	return teamID.String(), true, nil
}

func replayOrReject(ctx context.Context, c *echo.Context, repository idempotencyRepository, scope string, key string, requestHash string, now time.Time) error {
	record, err := repository.Get(ctx, scope, key)
	if err != nil {
		return httputil.Error(c, apperrors.NewInternal("Unable to load idempotency key", err))
	}

	if record.RequestHash != requestHash {
		return httputil.Error(
			c,
			apperrors.NewConflict("Idempotency-Key was already used with a different request"),
		)
	}

	if record.Status == idempotency.StatusCompleted && record.ResponseStatus != nil {
		if err := restoreResponseHeaders(c.Response().Header(), record.ResponseHeaders); err != nil {
			return httputil.Error(
				c,
				apperrors.NewInternal("Unable to replay idempotent response headers", err),
			)
		}
		if record.ResponseContentType != nil && *record.ResponseContentType != "" {
			c.Response().Header().Set(echo.HeaderContentType, *record.ResponseContentType)
		}

		c.Response().WriteHeader(int(*record.ResponseStatus))
		_, err := c.Response().Write(record.ResponseBody)
		return err
	}

	if record.LockedUntil.After(now) {
		return httputil.Error(
			c,
			apperrors.NewConflict("Idempotency-Key request is still processing"),
		)
	}

	_ = repository.Delete(ctx, scope, key)
	return httputil.Error(
		c,
		apperrors.NewConflict("Idempotency-Key expired while processing; retry the request"),
	)
}

func isIdempotencyCandidate(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func isCookieSessionRoute(path string) bool {
	switch path {
	case "/auth/register", "/auth/login", "/auth/logout":
		return true
	default:
		return false
	}
}

func requestIdempotencyScope(request *http.Request) (string, bool) {
	cookie, err := request.Cookie(authnz.SessionCookieName)
	if err == nil && strings.TrimSpace(cookie.Value) != "" {
		return credentialScope("session", cookie.Value), true
	}

	if token, ok := parseBearerToken(request.Header.Get(echo.HeaderAuthorization)); ok {
		return credentialScope("bearer", token), true
	}

	return "", false
}

func credentialScope(kind string, credential string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(kind))
	_, _ = hash.Write([]byte("\n"))
	_, _ = hash.Write([]byte(credential))
	return kind + ":" + hex.EncodeToString(hash.Sum(nil))
}

func readAndRestoreBody(request *http.Request) ([]byte, error) {
	if request.Body == nil {
		return nil, nil
	}

	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}

	request.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

func hashRequest(method string, path string, query string, body []byte) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(method))
	_, _ = hash.Write([]byte("\n"))
	_, _ = hash.Write([]byte(path))
	_, _ = hash.Write([]byte("\n"))
	_, _ = hash.Write([]byte(query))
	_, _ = hash.Write([]byte("\n"))
	_, _ = hash.Write(body)
	return hex.EncodeToString(hash.Sum(nil))
}

func encodeResponseHeaders(headers http.Header) ([]byte, error) {
	replayable := make(http.Header)
	for name, values := range headers {
		canonicalName := http.CanonicalHeaderKey(name)
		if _, excluded := nonReplayableResponseHeaders[canonicalName]; excluded {
			continue
		}
		replayable[canonicalName] = append([]string(nil), values...)
	}

	return json.Marshal(replayable)
}

func restoreResponseHeaders(destination http.Header, encoded []byte) error {
	if len(encoded) == 0 {
		return nil
	}

	var stored http.Header
	if err := json.Unmarshal(encoded, &stored); err != nil {
		return err
	}

	for name, values := range stored {
		canonicalName := http.CanonicalHeaderKey(name)
		if _, excluded := nonReplayableResponseHeaders[canonicalName]; excluded {
			continue
		}
		destination.Del(canonicalName)
		for _, value := range values {
			destination.Add(canonicalName, value)
		}
	}

	return nil
}

type responseRecorder struct {
	http.ResponseWriter
	body   bytes.Buffer
	status int
}

func newResponseRecorder(response http.ResponseWriter) *responseRecorder {
	return &responseRecorder{ResponseWriter: response}
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	if r.status != 0 {
		return
	}

	r.status = statusCode
}

func (r *responseRecorder) Write(body []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}

	return r.body.Write(body)
}

func (r *responseRecorder) Flush() error {
	status := r.status
	if status == 0 {
		status = http.StatusOK
	}

	r.ResponseWriter.WriteHeader(status)
	_, err := r.ResponseWriter.Write(r.body.Bytes())
	return err
}

func (r *responseRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
