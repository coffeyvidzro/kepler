package broadcastexecution

import (
	"context"
	"errors"
	"testing"

	broadcastmodule "github.com/coffeyvidzro/dugble/server/internal/modules/broadcast"
)

type fakeRepository struct {
	results []claimResult
	calls   int
}

type claimResult struct {
	broadcast broadcastmodule.Broadcast
	claimed   bool
	err       error
}

func (repository *fakeRepository) QueueNextDueScheduled(context.Context) (broadcastmodule.Broadcast, bool, error) {
	repository.calls++
	if len(repository.results) == 0 {
		return broadcastmodule.Broadcast{}, false, nil
	}
	result := repository.results[0]
	repository.results = repository.results[1:]
	return result.broadcast, result.claimed, result.err
}

func TestPollStopsWhenNoBroadcastIsDue(t *testing.T) {
	repository := &fakeRepository{results: []claimResult{
		{broadcast: broadcastmodule.Broadcast{ID: "first"}, claimed: true},
		{claimed: false},
	}}
	consumer := NewConsumer(repository, Config{BatchSize: 10})

	if err := consumer.poll(context.Background()); err != nil {
		t.Fatalf("poll returned error: %v", err)
	}
	if repository.calls != 2 {
		t.Fatalf("expected 2 claim calls, got %d", repository.calls)
	}
}

func TestPollHonorsBatchSize(t *testing.T) {
	repository := &fakeRepository{results: []claimResult{
		{broadcast: broadcastmodule.Broadcast{ID: "first"}, claimed: true},
		{broadcast: broadcastmodule.Broadcast{ID: "second"}, claimed: true},
		{broadcast: broadcastmodule.Broadcast{ID: "third"}, claimed: true},
	}}
	consumer := NewConsumer(repository, Config{BatchSize: 2})

	if err := consumer.poll(context.Background()); err != nil {
		t.Fatalf("poll returned error: %v", err)
	}
	if repository.calls != 2 {
		t.Fatalf("expected 2 claim calls, got %d", repository.calls)
	}
}

func TestPollReturnsClaimError(t *testing.T) {
	expected := errors.New("database unavailable")
	repository := &fakeRepository{results: []claimResult{{err: expected}}}
	consumer := NewConsumer(repository, Config{BatchSize: 10})

	if err := consumer.poll(context.Background()); !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}
