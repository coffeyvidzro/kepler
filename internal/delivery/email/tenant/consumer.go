package tenantprovision

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

	jetstreammessaging "github.com/coffeyvidzro/dugble/server/internal/adapters/nats"
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

type commandProcessor interface {
	Handle(context.Context, Command) error
	HandleExhausted(context.Context, Command, error) error
}

type Config struct {
	Concurrency    int
	AckWait        time.Duration
	HandlerTimeout time.Duration
	MaxDeliver     int
	RetryBackOff   []time.Duration
}

type Consumer struct {
	provider  consumerProvider
	publisher messagePublisher
	processed processedEventStore
	processor commandProcessor
	config    Config
}

func NewConsumer(client *jetstreammessaging.Client, processed processedEventStore, processor commandProcessor, config Config) *Consumer {
	if config.Concurrency <= 0 {
		config.Concurrency = 3
	}
	if config.AckWait <= 0 {
		config.AckWait = 2 * time.Minute
	}
	if config.HandlerTimeout <= 0 {
		config.HandlerTimeout = 60 * time.Second
	}
	if len(config.RetryBackOff) == 0 {
		config.RetryBackOff = DefaultRetryBackOff()
	} else {
		config.RetryBackOff = append([]time.Duration(nil), config.RetryBackOff...)
	}
	if config.MaxDeliver <= 0 {
		config.MaxDeliver = len(config.RetryBackOff)
	}
	config.RetryBackOff = normalizeRetryBackOff(config.RetryBackOff, config.MaxDeliver)
	return &Consumer{provider: client, publisher: client, processed: processed, processor: processor, config: config}
}

func (consumer *Consumer) Run(ctx context.Context) error {
	if consumer == nil || consumer.provider == nil || consumer.publisher == nil || consumer.processed == nil || consumer.processor == nil {
		return ErrConsumerNotConfigured
	}
	streamConsumer, err := consumer.provider.CreateOrUpdateConsumer(ctx, jetstreammessaging.JobsStreamName, natsjs.ConsumerConfig{
		Name: ConsumerName, Durable: ConsumerName, Description: "Durable SES tenant provisioning jobs",
		DeliverPolicy: natsjs.DeliverAllPolicy, AckPolicy: natsjs.AckExplicitPolicy, AckWait: consumer.config.AckWait,
		MaxDeliver: consumer.config.MaxDeliver, BackOff: consumer.config.RetryBackOff,
		FilterSubject: ProvisionSubject, ReplayPolicy: natsjs.ReplayInstantPolicy,
		MaxAckPending: consumer.config.Concurrency * 4, MaxWaiting: consumer.config.Concurrency * 2, MaxRequestBatch: 1,
	})
	if err != nil {
		return fmt.Errorf("provision email tenant consumer: %w", err)
	}
	contexts := make([]natsjs.ConsumeContext, 0, consumer.config.Concurrency)
	for worker := range consumer.config.Concurrency {
		active, consumeErr := streamConsumer.Consume(func(message natsjs.Msg) { consumer.process(ctx, message) }, natsjs.PullMaxMessages(1))
		if consumeErr != nil {
			for _, item := range contexts {
				item.Stop()
			}
			return fmt.Errorf("start email tenant consumer worker %d: %w", worker, consumeErr)
		}
		contexts = append(contexts, active)
	}
	<-ctx.Done()
	for _, active := range contexts {
		active.Drain()
	}
	return nil
}

func (consumer *Consumer) process(parent context.Context, message natsjs.Msg) {
	metadata, err := message.Metadata()
	if err != nil {
		_ = message.Nak()
		return
	}
	command, err := decodeCommand(message)
	if err != nil {
		consumer.deadLetter(parent, message, metadata, uuid.Nil, err)
		return
	}
	processed, err := consumer.processed.IsProcessed(parent, ConsumerName, command.EventID)
	if err != nil {
		_ = message.Nak()
		return
	}
	if processed {
		_ = message.Ack()
		return
	}

	processorCtx, cancel := context.WithTimeout(parent, consumer.config.HandlerTimeout)
	err = consumer.processor.Handle(processorCtx, command)
	cancel()
	if err != nil {
		if parent.Err() != nil {
			return
		}
		if int(metadata.NumDelivered) >= consumer.config.MaxDeliver {
			finalizeCtx, finalizeCancel := context.WithTimeout(parent, consumer.config.HandlerTimeout)
			finalizeErr := consumer.processor.HandleExhausted(finalizeCtx, command, err)
			finalizeCancel()
			if finalizeErr != nil {
				_ = message.NakWithDelay(time.Minute)
				return
			}
			consumer.deadLetter(parent, message, metadata, command.EventID, err)
			return
		}
		_ = message.NakWithDelay(retryDelay(consumer.config.RetryBackOff, metadata.NumDelivered))
		return
	}

	if err := consumer.processed.MarkProcessed(parent, ConsumerName, command.EventID, map[string]any{
		"subject": message.Subject(), "stream_sequence": metadata.Sequence.Stream, "deliveries": metadata.NumDelivered,
	}); err != nil {
		_ = message.Nak()
		return
	}
	_ = message.Ack()
}

func decodeCommand(message natsjs.Msg) (Command, error) {
	var command Command
	if err := json.Unmarshal(message.Data(), &command); err != nil {
		return Command{}, fmt.Errorf("decode email tenant provisioning command: %w", err)
	}
	if err := ValidateCommand(command); err != nil {
		return Command{}, err
	}
	headerID, err := uuid.Parse(strings.TrimSpace(message.Headers().Get("Dugble-Event-Id")))
	if err != nil || headerID != command.EventID {
		return Command{}, errors.New("email tenant event ID does not match outbox header")
	}
	return command, nil
}

func (consumer *Consumer) deadLetter(ctx context.Context, message natsjs.Msg, metadata *natsjs.MsgMetadata, eventID uuid.UUID, cause error) {
	headers := map[string]string{
		"Dugble-Original-Subject":   message.Subject(),
		"Dugble-Dead-Letter-Reason": truncateReason(cause),
		"Dugble-Delivery-Count":     strconv.FormatUint(metadata.NumDelivered, 10),
	}
	messageID := eventID.String() + "-dlq"
	if eventID == uuid.Nil {
		messageID = fmt.Sprintf("%s-%d-dlq", ConsumerName, metadata.Sequence.Stream)
	}
	if err := consumer.publisher.Publish(ctx, DLQSubject, message.Data(), headers, messageID); err != nil {
		_ = message.NakWithDelay(time.Minute)
		return
	}
	if err := message.TermWithReason(truncateReason(cause)); err != nil {
		slog.Error("failed to terminate dead-lettered tenant provisioning command", "event_id", eventID, "error", err)
	}
}

func truncateReason(err error) string {
	if err == nil {
		return "unknown email tenant provisioning failure"
	}
	reason := strings.TrimSpace(err.Error())
	if len(reason) > 512 {
		return reason[:512]
	}
	if reason == "" {
		return "unknown email tenant provisioning failure"
	}
	return reason
}
