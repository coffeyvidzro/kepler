package workerhealth

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coffeyvidzro/dugble/server/internal/monitoring/verifymetrics"
	"github.com/coffeyvidzro/dugble/server/internal/worker"
)

type Dependency interface {
	Ping(context.Context) error
}

type Readiness interface {
	Ready() bool
	Snapshot() map[string]worker.ComponentState
	Policy() worker.FailurePolicy
}

type Handler struct {
	postgres  Dependency
	jetstream Dependency
	worker    Readiness
}

func NewHandler(postgres Dependency, jetstream Dependency, readiness Readiness) *Handler {
	return &Handler{postgres: postgres, jetstream: jetstream, worker: readiness}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.live)
	mux.HandleFunc("GET /ready", h.ready)
	mux.Handle("GET /metrics/verify", verifymetrics.Default)
	return mux
}

func (h *Handler) live(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, map[string]any{"status": "ok", "service": "dugble-worker"})
}

func (h *Handler) ready(response http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), 3*time.Second)
	defer cancel()

	checks := map[string]string{}
	postgresReady := check(ctx, h.postgres, checks, "postgres")
	jetstreamReady := check(ctx, h.jetstream, checks, "jetstream")
	componentsReady := h.worker != nil && h.worker.Ready()
	if componentsReady {
		checks["components"] = "ok"
	} else {
		checks["components"] = "unavailable"
	}

	status := http.StatusOK
	readiness := "ready"
	if !postgresReady || !jetstreamReady || !componentsReady {
		status = http.StatusServiceUnavailable
		readiness = "not_ready"
	}

	result := map[string]any{"status": readiness, "checks": checks}
	if h.worker != nil {
		result["failure_policy"] = h.worker.Policy()
		result["components"] = h.worker.Snapshot()
	}
	writeJSON(response, status, result)
}

func check(ctx context.Context, dependency Dependency, checks map[string]string, name string) bool {
	if dependency == nil {
		checks[name] = "unconfigured"
		return false
	}
	if err := dependency.Ping(ctx); err != nil {
		checks[name] = "unavailable"
		return false
	}
	checks[name] = "ok"
	return true
}

func writeJSON(response http.ResponseWriter, status int, body any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(body)
}
