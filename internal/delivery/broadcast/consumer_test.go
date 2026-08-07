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

func TestPollQueuesAndMaterializesUntilEmpty(t *testing.T) {
	repository := &fakeRepository{
		queueResults: []claimResult{
			{broadcast: broadcastmodule.Broadcast{ID: "first"}, claimed: true},
			{claimed: false},
		},
		materializeResults: []materializeResult{
			{result: broadcastmodule.MaterializationResult{AudienceCount: 3}, claimed: true},
			{claimed: false},
		},
	}
	consumer := NewConsumer(repository, Config{BatchSize: 10})

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

func TestPollHonorsBatchSizeForBothPhases(t *testing.T) {
	repository := &fakeRepository{
		queueResults: []claimResult{
			{claimed: true}, {claimed: true}, {claimed: true},
		},
		materializeResults: []materializeResult{
			{claimed: true}, {claimed: true}, {claimed: true},
		},
	}
	consumer := NewConsumer(repository, Config{BatchSize: 2})

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
	consumer := NewConsumer(repository, Config{BatchSize: 10})

	if err := consumer.poll(context.Background()); !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
	if repository.materializeCalls != 0 {
		t.Fatalf("expected materialization not to run after queue error")
	}
}

func TestPollReturnsMaterializationError(t *testing.T) {
	expected := errors.New("materialization failed")
	repository := &fakeRepository{
		queueResults:       []claimResult{{claimed: false}},
		materializeResults: []materializeResult{{err: expected}},
	}
	consumer := NewConsumer(repository, Config{BatchSize: 10})

	if err := consumer.poll(context.Background()); !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}
