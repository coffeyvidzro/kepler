package jetstream

import (
	"time"

	natsjs "github.com/nats-io/nats.go/jetstream"
)

const (
	JobsStreamName   = "DUGBLE_JOBS"
	EventsStreamName = "DUGBLE_EVENTS"
	DLQStreamName    = "DUGBLE_DLQ"

	JobsSubject   = "dugble.job.>"
	EventsSubject = "dugble.event.>"
	DLQSubject    = "dugble.dlq.>"
)

const maxMessageSize = 64 * 1024

type StreamLimits struct {
	JobsMaxBytes   int64
	EventsMaxBytes int64
	DLQMaxBytes    int64
	JobsMaxAge     time.Duration
	EventsMaxAge   time.Duration
	DLQMaxAge      time.Duration
}

func DefaultStreamLimits() StreamLimits {
	return StreamLimits{
		JobsMaxBytes:   5 * 1024 * 1024 * 1024,
		EventsMaxBytes: 10 * 1024 * 1024 * 1024,
		DLQMaxBytes:    5 * 1024 * 1024 * 1024,
		JobsMaxAge:     7 * 24 * time.Hour,
		EventsMaxAge:   30 * 24 * time.Hour,
		DLQMaxAge:      90 * 24 * time.Hour,
	}
}

func StreamConfigs(limits StreamLimits) []natsjs.StreamConfig {
	defaults := DefaultStreamLimits()
	if limits.JobsMaxBytes <= 0 {
		limits.JobsMaxBytes = defaults.JobsMaxBytes
	}
	if limits.EventsMaxBytes <= 0 {
		limits.EventsMaxBytes = defaults.EventsMaxBytes
	}
	if limits.DLQMaxBytes <= 0 {
		limits.DLQMaxBytes = defaults.DLQMaxBytes
	}
	if limits.JobsMaxAge <= 0 {
		limits.JobsMaxAge = defaults.JobsMaxAge
	}
	if limits.EventsMaxAge <= 0 {
		limits.EventsMaxAge = defaults.EventsMaxAge
	}
	if limits.DLQMaxAge <= 0 {
		limits.DLQMaxAge = defaults.DLQMaxAge
	}
	return []natsjs.StreamConfig{
		{
			Name:        JobsStreamName,
			Description: "Durable Dugble background jobs",
			Subjects:    []string{JobsSubject},
			Retention:   natsjs.WorkQueuePolicy,
			Discard:     natsjs.DiscardNew,
			Storage:     natsjs.FileStorage,
			Replicas:    3,
			MaxBytes:    limits.JobsMaxBytes,
			MaxAge:      limits.JobsMaxAge,
			MaxMsgSize:  maxMessageSize,
			Duplicates:  10 * time.Minute,
		},
		{
			Name:        EventsStreamName,
			Description: "Replayable Dugble domain and provider events",
			Subjects:    []string{EventsSubject},
			Retention:   natsjs.LimitsPolicy,
			Discard:     natsjs.DiscardOld,
			Storage:     natsjs.FileStorage,
			Replicas:    3,
			MaxBytes:    limits.EventsMaxBytes,
			MaxAge:      limits.EventsMaxAge,
			MaxMsgSize:  maxMessageSize,
			Duplicates:  10 * time.Minute,
		},
		{
			Name:        DLQStreamName,
			Description: "Dugble jobs requiring operator inspection or redrive",
			Subjects:    []string{DLQSubject},
			Retention:   natsjs.LimitsPolicy,
			Discard:     natsjs.DiscardOld,
			Storage:     natsjs.FileStorage,
			Replicas:    3,
			MaxBytes:    limits.DLQMaxBytes,
			MaxAge:      limits.DLQMaxAge,
			MaxMsgSize:  maxMessageSize,
			Duplicates:  10 * time.Minute,
		},
	}
}
