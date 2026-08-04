package feedback

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

type ObservedReconciler struct {
	reconciler *Reconciler
	metrics    *Metrics
}

func NewObservedReconciler(repository *Repository, config ReconcilerConfig, metrics *Metrics) *ObservedReconciler {
	if metrics == nil {
		metrics = DefaultMetrics
	}
	return &ObservedReconciler{
		reconciler: NewReconciler(repository, config),
		metrics:    metrics,
	}
}

func (r *ObservedReconciler) Run(ctx context.Context) error {
	if r == nil || r.reconciler == nil || r.reconciler.repository == nil || r.metrics == nil {
		return errors.New("observed email feedback reconciler is not configured")
	}
	ticker := time.NewTicker(r.reconciler.config.PollInterval)
	defer ticker.Stop()
	for {
		startedAt := time.Now()
		claims, err := r.reconciler.repository.ClaimDue(ctx, r.reconciler.config.BatchSize, r.reconciler.config.LeaseDuration)
		r.metrics.ObserveOperation("reconcile_claim", time.Since(startedAt))
		r.metrics.SetLastClaimedBatch(len(claims))
		if err != nil {
			r.metrics.RecordEvent("reconcile", "batch", "claim_error")
			if ctx.Err() == nil {
				slog.Error("email feedback reconciliation claim failed", "error", err)
			}
		} else if len(claims) == 0 {
			r.metrics.RecordEvent("reconcile", "batch", "empty")
		} else {
			r.metrics.RecordEvent("reconcile", "batch", "claimed")
			batchStartedAt := time.Now()
			if err := r.processClaims(ctx, claims); err != nil && ctx.Err() == nil {
				slog.Error("email feedback reconciliation batch failed", "error", err)
			}
			r.metrics.ObserveOperation("reconcile_process", time.Since(batchStartedAt))
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (r *ObservedReconciler) processClaims(ctx context.Context, claims []ReconcileClaim) error {
	for _, claim := range claims {
		startedAt := time.Now()
		handleCtx, cancel := context.WithTimeout(ctx, r.reconciler.config.HandleTimeout)
		err := r.reconciler.repository.processClaimed(handleCtx, claim)
		cancel()
		r.metrics.ObserveOperation("reconcile_event", time.Since(startedAt))
		if err == nil {
			r.metrics.RecordEvent("reconcile", "provider_event", "processed")
			continue
		}
		if recordErr := r.reconciler.repository.RecordReconcileFailure(ctx, claim, err); recordErr != nil {
			r.metrics.RecordEvent("reconcile", "provider_event", "persist_failure")
			return recordErr
		}
		if claim.AttemptCount >= defaultReconciliationMaxAttempts {
			r.metrics.RecordEvent("reconcile", "provider_event", "dead_lettered")
		} else {
			r.metrics.RecordEvent("reconcile", "provider_event", "rescheduled")
		}
	}
	return nil
}
