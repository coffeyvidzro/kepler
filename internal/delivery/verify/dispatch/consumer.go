package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	natsjs "github.com/nats-io/nats.go/jetstream"

	jetstreammessaging "github.com/coffeyvidzro/dugble/server/internal/messaging/jetstream"
)

const (
	ConsumerName = "dugble-verify-dispatch-v1"
	DLQSubject   = "dugble.dlq.verify.dispatch.v1"
)

type processedEventStore interface {
	IsProcessed(context.Context, string, uuid.UUID) (bool, error)
	MarkProcessed(context.Context, string, uuid.UUID, map[string]any) error
}
type consumerProvider interface {
	CreateOrUpdateConsumer(context.Context, string, natsjs.ConsumerConfig) (natsjs.Consumer, error)
}
type messagePublisher interface {
	Publish(context.Context, string, []byte, map[string]string, string) error
}
type dispatchHandler interface {
	Handle(context.Context, Command) error
	HandleExhausted(context.Context, Command, error) error
}

type Consumer struct {
	provider  consumerProvider
	publisher messagePublisher
	processed processedEventStore
	handler   dispatchHandler
	config    ConsumerConfig
}

func NewConsumer(client *jetstreammessaging.Client, processed processedEventStore, handler dispatchHandler, config ConsumerConfig) *Consumer {
	return &Consumer{provider: client, publisher: client, processed: processed, handler: handler, config: normalizeConsumerConfig(config)}
}

func (consumer *Consumer) Run(ctx context.Context) error {
	if consumer == nil || consumer.provider == nil || consumer.publisher == nil || consumer.processed == nil || consumer.handler == nil {
		return errors.New("verification dispatch consumer is not fully configured")
	}
	streamConsumer, err := consumer.provider.CreateOrUpdateConsumer(ctx, jetstreammessaging.JobsStreamName, natsjs.ConsumerConfig{
		Name: ConsumerName, Durable: ConsumerName, Description: "Durable verification challenge dispatch commands",
		DeliverPolicy: natsjs.DeliverAllPolicy, AckPolicy: natsjs.AckExplicitPolicy, AckWait: consumer.config.AckWait,
		MaxDeliver: consumer.config.MaxDeliver, BackOff: append([]time.Duration(nil), consumer.config.RetryDelays...),
		FilterSubject: Subject, ReplayPolicy: natsjs.ReplayInstantPolicy,
		MaxAckPending: max(consumer.config.Concurrency*4, consumer.config.Concurrency), MaxWaiting: max(consumer.config.Concurrency*2, consumer.config.Concurrency), MaxRequestBatch: 1,
	})
	if err != nil {
		return fmt.Errorf("provision verification dispatch consumer: %w", err)
	}
	contexts := make([]natsjs.ConsumeContext, 0, consumer.config.Concurrency)
	errorsChannel := make(chan error, consumer.config.Concurrency)
	for index := range consumer.config.Concurrency {
		active, consumeErr := streamConsumer.Consume(func(message natsjs.Msg) { consumer.processMessage(ctx, message) }, natsjs.PullMaxMessages(1), natsjs.ConsumeErrHandler(func(_ natsjs.ConsumeContext, err error) {
			if err != nil {
				slog.Warn("verification dispatch consumer error", "worker", index, "error", err)
			}
		}))
		if consumeErr != nil {
			for _, item := range contexts {
				item.Stop()
			}
			return fmt.Errorf("start verification dispatch worker %d: %w", index, consumeErr)
		}
		contexts = append(contexts, active)
		go func(worker int, item natsjs.ConsumeContext) {
			<-item.Closed()
			if ctx.Err() == nil {
				select {
				case errorsChannel <- fmt.Errorf("verification dispatch worker %d stopped unexpectedly", worker):
				default:
				}
			}
		}(index, active)
	}
	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-errorsChannel:
	}
	for _, item := range contexts {
		item.Drain()
	}
	for _, item := range contexts {
		select {
		case <-item.Closed():
		case <-time.After(consumer.config.HandlerTimeout):
			item.Stop()
		}
	}
	return runErr
}

func (consumer *Consumer) processMessage(parent context.Context, message natsjs.Msg) {
	metadata, err := message.Metadata()
	if err != nil {
		consumer.retry(message, 1, err)
		return
	}
	command, eventID, err := decodeCommand(message)
	if err != nil {
		consumer.deadLetter(parent, message, metadata, uuid.Nil, err)
		return
	}
	processed, err := consumer.processed.IsProcessed(parent, ConsumerName, eventID)
	if err != nil {
		consumer.retry(message, metadata.NumDelivered, err)
		return
	}
	if processed {
		consumer.ack(parent, message, eventID)
		return
	}
	handlerCtx, cancel := context.WithTimeout(parent, consumer.config.HandlerTimeout)
	err = consumer.handler.Handle(handlerCtx, command)
	cancel()
	if err != nil {
		if parent.Err() != nil {
			return
		}
		if int(metadata.NumDelivered) >= consumer.config.MaxDeliver {
			finalCtx, finalCancel := context.WithTimeout(parent, consumer.config.HandlerTimeout)
			finalErr := consumer.handler.HandleExhausted(finalCtx, command, err)
			finalCancel()
			if finalErr != nil {
				consumer.retry(message, metadata.NumDelivered, errors.Join(err, finalErr))
				return
			}
			consumer.deadLetter(parent, message, metadata, eventID, err)
			return
		}
		consumer.retry(message, metadata.NumDelivered, err)
		return
	}
	if err := consumer.processed.MarkProcessed(parent, ConsumerName, eventID, map[string]any{"subject": message.Subject(), "stream_sequence": metadata.Sequence.Stream, "deliveries": metadata.NumDelivered}); err != nil {
		consumer.retry(message, metadata.NumDelivered, err)
		return
	}
	consumer.ack(parent, message, eventID)
}

func decodeCommand(message natsjs.Msg) (Command, uuid.UUID, error) {
	var command Command
	if err := json.Unmarshal(message.Data(), &command); err != nil {
		return Command{}, uuid.Nil, fmt.Errorf("decode verification dispatch command: %w", err)
	}
	if err := ValidateCommand(command); err != nil {
		return Command{}, uuid.Nil, err
	}
	eventID := EventID(command.ChallengeID)
	headerID, err := uuid.Parse(strings.TrimSpace(message.Headers().Get("Dugble-Event-Id")))
	if err != nil || headerID != eventID {
		return Command{}, uuid.Nil, errors.New("verification dispatch event ID does not match the outbox header")
	}
	return command, eventID, nil
}

func (consumer *Consumer) retry(message natsjs.Msg, delivered uint64, cause error) {
	index := max(int(delivered)-1, 0)
	if index >= len(consumer.config.RetryDelays) {
		index = len(consumer.config.RetryDelays) - 1
	}
	delay := consumer.config.RetryDelays[index]
	if err := message.NakWithDelay(delay); err != nil {
		slog.Error("failed to retry verification dispatch", "error", err, "cause", cause)
		return
	}
	slog.Warn("verification dispatch will be retried", "delivery", delivered, "delay", delay, "error", cause)
}

func (consumer *Consumer) ack(parent context.Context, message natsjs.Msg, eventID uuid.UUID) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	if err := message.DoubleAck(ctx); err != nil {
		slog.Warn("failed to acknowledge verification dispatch", "event_id", eventID, "error", err)
	}
}

func (consumer *Consumer) deadLetter(ctx context.Context, message natsjs.Msg, metadata *natsjs.MsgMetadata, eventID uuid.UUID, cause error) {
	headers := make(map[string]string, len(message.Headers())+3)
	for key, values := range message.Headers() {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	headers["Dugble-Original-Subject"] = message.Subject()
	headers["Dugble-Dead-Letter-Reason"] = truncateReason(cause)
	headers["Dugble-Delivery-Count"] = strconv.FormatUint(metadata.NumDelivered, 10)
	messageID := eventID.String() + "-dlq"
	if eventID == uuid.Nil {
		messageID = fmt.Sprintf("%s-%d-dlq", ConsumerName, metadata.Sequence.Stream)
	}
	if err := consumer.publisher.Publish(ctx, DLQSubject, message.Data(), headers, messageID); err != nil {
		consumer.retry(message, metadata.NumDelivered, fmt.Errorf("publish verification dispatch DLQ: %w", err))
		return
	}
	if err := message.TermWithReason(truncateReason(cause)); err != nil {
		slog.Error("failed to terminate verification dispatch", "event_id", eventID, "error", err)
	}
}

func truncateReason(err error) string {
	if err == nil {
		return "unknown verification dispatch failure"
	}
	value := strings.TrimSpace(err.Error())
	if len(value) > 512 {
		return value[:512]
	}
	return value
}
