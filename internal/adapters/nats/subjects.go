package nats

import legacy "github.com/coffeyvidzro/dugble/server/internal/messaging/jetstream"

const (
	JobsStreamName   = legacy.JobsStreamName
	EventsStreamName = legacy.EventsStreamName
	DLQStreamName    = legacy.DLQStreamName
	JobsSubject      = legacy.JobsSubject
	EventsSubject    = legacy.EventsSubject
	DLQSubject       = legacy.DLQSubject
)
