package feedback

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type eventProcessor interface {
	Process(context.Context, uuid.UUID) error
}

type Handler struct {
	repository eventProcessor
	metrics    *Metrics
}

func NewHandler(repository eventProcessor) *Handler {
	return NewHandlerWithMetrics(repository, DefaultMetrics)
}

func NewHandlerWithMetrics(repository eventProcessor, metrics *Metrics) *Handler {
	if metrics == nil {
		metrics = DefaultMetrics
	}
	return &Handler{repository: repository, metrics: metrics}
}

func (h *Handler) Handle(ctx context.Context, event ProviderEventReference) error {
	if h == nil || h.repository == nil {
		return errors.New("email feedback handler is not configured")
	}
	if event.EventID == uuid.Nil {
		return errors.New("email feedback event ID is required")
	}
	if strings.TrimSpace(event.Provider) != ProviderSES {
		return errors.New("unsupported email feedback provider")
	}
	startedAt := time.Now()
	err := h.repository.Process(ctx, event.EventID)
	h.metrics.ObserveOperation("process", time.Since(startedAt))
	if err != nil {
		h.metrics.RecordEvent("process", "provider_event", "error")
		return err
	}
	h.metrics.RecordEvent("process", "provider_event", "success")
	return nil
}
