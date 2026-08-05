package nats

const (
	JobsStreamName   = "DUGBLE_JOBS"
	EventsStreamName = "DUGBLE_EVENTS"
	DLQStreamName    = "DUGBLE_DLQ"

	JobsSubject   = "dugble.job.>"
	EventsSubject = "dugble.event.>"
	DLQSubject    = "dugble.dlq.>"
)
