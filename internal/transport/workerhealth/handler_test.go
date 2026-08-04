package workerhealth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coffeyvidzro/dugble/server/internal/worker"
)

type dependencyStub struct{ err error }

func (stub dependencyStub) Ping(context.Context) error { return stub.err }

type readinessStub struct{ ready bool }

func (stub readinessStub) Ready() bool                  { return stub.ready }
func (stub readinessStub) Policy() worker.FailurePolicy { return worker.FailFast }
func (stub readinessStub) Snapshot() map[string]worker.ComponentState {
	return map[string]worker.ComponentState{"consumer": {Status: worker.StatusRunning}}
}

func TestLive(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()
	NewHandler(nil, nil, nil).Routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"service":"dugble-worker"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestReady(t *testing.T) {
	tests := []struct {
		name       string
		postgres   Dependency
		jetstream  Dependency
		readiness  Readiness
		wantStatus int
		wantBody   string
	}{
		{name: "ready", postgres: dependencyStub{}, jetstream: dependencyStub{}, readiness: readinessStub{ready: true}, wantStatus: http.StatusOK, wantBody: `"status":"ready"`},
		{name: "dependency unavailable", postgres: dependencyStub{err: errors.New("down")}, jetstream: dependencyStub{}, readiness: readinessStub{ready: true}, wantStatus: http.StatusServiceUnavailable, wantBody: `"postgres":"unavailable"`},
		{name: "component unavailable", postgres: dependencyStub{}, jetstream: dependencyStub{}, readiness: readinessStub{}, wantStatus: http.StatusServiceUnavailable, wantBody: `"components":"unavailable"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/ready", nil)
			response := httptest.NewRecorder()
			NewHandler(test.postgres, test.jetstream, test.readiness).Routes().ServeHTTP(response, request)
			if response.Code != test.wantStatus || !strings.Contains(response.Body.String(), test.wantBody) {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
		})
	}
}
