package audit

import (
	"context"
	"log/slog"
	"sync/atomic"
)

type Sink interface {
	Record(context.Context, Entry) error
}

type sinkHolder struct {
	sink Sink
}

var configuredSink atomic.Pointer[sinkHolder]

func SetSink(sink Sink) {
	if sink == nil {
		configuredSink.Store(nil)
		return
	}
	configuredSink.Store(&sinkHolder{sink: sink})
}

func persist(ctx context.Context, entry Entry) {
	if ctx == nil {
		ctx = context.Background()
	}
	entry.Outcome = normalizedOutcome(entry.Outcome)
	entry.Metadata = normalizedMetadata(entry.Metadata)
	if metadata, ok := RequestMetadataFromContext(ctx); ok {
		entry.Request = metadata
	}
	if holder := configuredSink.Load(); holder != nil {
		if err := holder.sink.Record(ctx, entry); err != nil {
			slog.ErrorContext(
				ctx,
				"failed to persist security audit event",
				"audit_action",
				entry.Action,
				"error",
				err,
			)
		}
	}
}
