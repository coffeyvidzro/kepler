package expiry

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeBatchExpirer struct {
	mu      sync.Mutex
	results []int
	calls   int
	err     error
}

func (fake *fakeBatchExpirer) ExpireBatch(context.Context, int32) (int, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.calls++
	if fake.err != nil {
		return 0, fake.err
	}
	if len(fake.results) == 0 {
		return 0, nil
	}
	result := fake.results[0]
	fake.results = fake.results[1:]
	return result, nil
}

func TestPollDrainsFullBatches(t *testing.T) {
	repository := &fakeBatchExpirer{results: []int{2, 2, 1}}
	consumer := NewConsumer(repository, Config{BatchSize: 2, BatchTimeout: time.Second})
	consumer.poll(context.Background())
	if repository.calls != 3 {
		t.Fatalf("ExpireBatch calls = %d, want 3", repository.calls)
	}
}

func TestPollStopsAfterFailure(t *testing.T) {
	repository := &fakeBatchExpirer{err: errors.New("database unavailable")}
	consumer := NewConsumer(repository, Config{BatchSize: 2, BatchTimeout: time.Second})
	consumer.poll(context.Background())
	if repository.calls != 1 {
		t.Fatalf("ExpireBatch calls = %d, want 1", repository.calls)
	}
}

func TestRunRejectsMissingRepository(t *testing.T) {
	consumer := NewConsumer(nil, DefaultConfig())
	if err := consumer.Run(context.Background()); err == nil {
		t.Fatal("Run() accepted a missing repository")
	}
}
