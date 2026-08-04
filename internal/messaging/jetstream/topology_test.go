package jetstream

import (
	"testing"

	natsjs "github.com/nats-io/nats.go/jetstream"
)

func TestStreamConfigs(t *testing.T) {
	t.Parallel()

	configs := StreamConfigs(StreamLimits{})
	if len(configs) != 3 {
		t.Fatalf("expected 3 streams, got %d", len(configs))
	}

	byName := make(map[string]natsjs.StreamConfig, len(configs))
	for _, config := range configs {
		byName[config.Name] = config
		if config.Storage != natsjs.FileStorage {
			t.Errorf("stream %s must use file storage", config.Name)
		}
		if config.Replicas != 3 {
			t.Errorf("stream %s must use three replicas", config.Name)
		}
		if config.MaxBytes <= 0 || config.MaxAge <= 0 || config.MaxMsgSize <= 0 {
			t.Errorf("stream %s must have explicit limits", config.Name)
		}
	}

	jobs := byName[JobsStreamName]
	if jobs.Retention != natsjs.WorkQueuePolicy || jobs.Discard != natsjs.DiscardNew {
		t.Fatalf("jobs stream must preserve unprocessed work and reject new messages at its limit")
	}
	if len(jobs.Subjects) != 1 || jobs.Subjects[0] != JobsSubject {
		t.Fatalf("unexpected jobs subjects: %v", jobs.Subjects)
	}

	events := byName[EventsStreamName]
	if events.Retention != natsjs.LimitsPolicy || events.Discard != natsjs.DiscardOld {
		t.Fatalf("events stream must retain a bounded replay window")
	}

	dlq := byName[DLQStreamName]
	if dlq.Retention != natsjs.LimitsPolicy || dlq.Discard != natsjs.DiscardOld {
		t.Fatalf("DLQ stream must retain a bounded investigation window")
	}
}
