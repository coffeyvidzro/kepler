package broadcastexecution

import (
	"context"
	"errors"
	"testing"

	broadcastmodule "github.com/coffeyvidzro/dugble/server/internal/modules/broadcast"
)

type fakeRepository struct {
	queueResults       []claimResult
	materializeResults []materializeResult
	queueCalls         int
	materializeCalls   int
}

type claimResult struct {
	broadcast broadcastmodule.Broadcast
	claimed   bool
	err       error
}

type materializeResult struct {
	result  broadcastmodule.MaterializationResult
	claimed bool
	err     error
}

func (repository *fakeRepository) QueueNextDueScheduled(context.Context) (broadcastmodule.Broadcast, bool, error) {
	repository.queueCalls++
	if len(repository.queueResults) == 0 {
		return broadcastmodule.Broadcast{}, false, nil
	}
	result := repository.queueResults[0]
	repository.queueResults = repository.queueResults[1:]
	return result.broadcast, result.claimed, result.err
}

func (repository *fakeRepository) MaterializeNextQueuedRecipients(context.Context) (broadcastmodule.MaterializationResult, bool, error) {
	repository.materializeCalls++
	if len(repository.materializeResults) == 0 {
		return broadcastmodule.MaterializationResult{}, false, nil
	}
	result := repository.materializeResults[0]
	repository.materializeResults = repository.materializeResults[1:]
	return result.result, result.claimed, result.err
}

func newTestConsumer(repository repository, batchSize int) *Consumer {
	return NewConsumer(NewProcessor(repository), Config{BatchSize: batchSize})
}

func TestPollStopsWhenNoWorkRemains(t *testing.T) {
	repository := &fakeRepository{
		queueResults: []claimResult{
			{broadcast: broadcastmodule.Broadcast{ID: "first"}, claimed: true},
			{claimed: false},
		},
		materializeResults: []materializeResult{
			{result: broadcastmodule.MaterializationResult{}, claimed: true},
			{claimed: false},
		},
	}
	consumer := newTestConsumer(repository, 10)

	if err := consumer.poll(context.Background()); err != nil {
		t.Fatalf("poll returned error: %v", err)
	}
	if repository.queueCalls != 2 {
		t.Fatalf("expected 2 queue calls, got %d", repository.queueCalls)
	}
	if repository.materializeCalls != 2 {
		t.Fatalf("expected 2 materialization calls, got %d", repository.materializeCalls)
	}
}

func TestPollHonorsBatchSize(t *testing.T) {
	repository := &fakeRepository{
		queueResults: []claimResult{
			{claimed: true},
			{claimed: true},
			{claimed: true},
		},
		materializeResults: []materializeResult{
			{claimed: true},
			{claimed: true},
			{claimed: true},
		},
	}
	consumer := newTestConsumer(repository, 2)

	if err := consumer.poll(context.Background()); err != nil {
		t.Fatalf("poll returned error: %v", err)
	}
	if repository.queueCalls != 2 {
		t.Fatalf("expected 2 queue calls, got %d", repository.queueCalls)
	}
	if repository.materializeCalls != 2 {
		t.Fatalf("expected 2 materialization calls, got %d", repository.materializeCalls)
	}
}

func TestPollReturnsQueueError(t *testing.T) {
	expected := errors.New("database unavailable")
	repository := &fakeRepository{queueResults: []claimResult{{err: expected}}}
	consumer := newTestConsumer(repository, 10)

	if err := consumer.poll(context.Background()); !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}

func TestPollReturnsMaterializationError(t *testing.T) {
	expected := errors.New("materialization failed")
	repository := &fakeRepository{
		queueResults:       []claimResult{{claimed: false}},
		materializeResults: []materializeResult{{err: expected}},
	}
	consumer := newTestConsumer(repository, 10)

	if err := consumer.poll(context.Background()); !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}
