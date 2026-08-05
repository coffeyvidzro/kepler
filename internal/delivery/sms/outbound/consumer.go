package smsdelivery

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

const maxDeadLetterReasonLength = 512

var defaultRetryDelays = []time.Duration{
	5 * time.Second,
	30 * time.Second,
	2 * time.Minute,
	10 * time.Minute,
	1 * time.Hour,
	6 * time.Hour,
}

type deliveryHandler interface {
	Handle(context.Context, DeliverCommand) error
	HandleExhausted(context.Context, DeliverCommand, error) error
}

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

type ConsumerConfig struct {
	Concurrency    int
	AckWait        time.Duration
	HandlerTimeout time.Duration
	MaxDeliver     int
	RetryDelays    []time.Duration
}

func DefaultConsumerConfig() ConsumerConfig {
	return ConsumerConfig{
		Concurrency:    10,
		AckWait:        2 * time.Minute,
		HandlerTimeout: 45 * time.Second,
		MaxDeliver:     len(defaultRetryDelays),
		RetryDelays:    append([]time.Duration(nil), defaultRetryDelays...),
	}
}

type Consumer struct {
	provider  consumerProvider
	publisher messagePublisher
	processed processedEventStore
	handler   deliveryHandler
	config    ConsumerConfig
}

func NewConsumer(
	provider *jetstreammessaging.Client,
	processed processedEventStore,
	handler deliveryHandler,
	config ConsumerConfig,
) *Consumer {
	return &Consumer{
		provider:  provider,
		publisher: provider,
		processed: processed,
		handler:   handler,
		config:    normalizeConsumerConfig(config),
	}
}

func normalizeConsumerConfig(config ConsumerConfig) ConsumerConfig {
	defaults := DefaultConsumerConfig()
	if config.Concurrency <= 0 {
		config.Concurrency = defaults.Concurrency
	}
	if config.AckWait <= 0 {
		config.AckWait = defaults.AckWait
	}
	if config.HandlerTimeout <= 0 {
		config.HandlerTimeout = defaults.HandlerTimeout
	}
	if config.MaxDeliver <= 0 {
		config.MaxDeliver = defaults.MaxDeliver
	}
	if len(config.RetryDelays) == 0 {
		config.RetryDelays = defaults.RetryDelays
	} else {
		config.RetryDelays = append([]time.Duration(nil), config.RetryDelays...)
	}
	for index, delay := range config.RetryDelays {
		if delay <= 0 {
			config.RetryDelays[index] = defaults.RetryDelays[min(index, len(defaults.RetryDelays)-1)]
		}
	}
	if len(config.RetryDelays) > config.MaxDeliver {
		config.RetryDelays = config.RetryDelays[:config.MaxDeliver]
	}
	return config
}

func (c *Consumer) Run(ctx context.Context) error {
	if c == nil || c.provider == nil {
		return errors.New("SMS JetStream consumer provider is not configured")
	}
	if c.publisher == nil {
		return errors.New("SMS dead-letter publisher is not configured")
	}
	if c.processed == nil {
		return errors.New("processed event store is not configured")
	}
	if c.handler == nil {
		return errors.New("SMS delivery handler is not configured")
	}

	consumer, err := c.provider.CreateOrUpdateConsumer(ctx, jetstreammessaging.JobsStreamName, natsjs.ConsumerConfig{
		Name:            DeliverConsumerName,
		Durable:         DeliverConsumerName,
		Description:     "Durable SMS delivery commands",
		DeliverPolicy:   natsjs.DeliverAllPolicy,
		AckPolicy:       natsjs.AckExplicitPolicy,
		AckWait:         c.config.AckWait,
		MaxDeliver:      c.config.MaxDeliver,
		BackOff:         append([]time.Duration(nil), c.config.RetryDelays...),
		FilterSubject:   DeliverSubject,
		ReplayPolicy:    natsjs.ReplayInstantPolicy,
		MaxAckPending:   max(c.config.Concurrency*4, c.config.Concurrency),
		MaxWaiting:      max(c.config.Concurrency*2, c.config.Concurrency),
		MaxRequestBatch: 1,
	})
	if err != nil {
		return fmt.Errorf("provision SMS delivery consumer: %w", err)
	}

	consumeContexts := make([]natsjs.ConsumeContext, 0, c.config.Concurrency)
	consumeErrors := make(chan error, c.config.Concurrency)
	for workerIndex := range c.config.Concurrency {
		consumeContext, consumeErr := consumer.Consume(
			func(message natsjs.Msg) {
				c.processMessage(ctx, message)
			},
			natsjs.PullMaxMessages(1),
			natsjs.ConsumeErrHandler(func(_ natsjs.ConsumeContext, consumeErr error) {
				if consumeErr != nil {
					slog.Warn("SMS consumer encountered a JetStream error", "worker", workerIndex, "error", consumeErr)
				}
			}),
		)
		if consumeErr != nil {
			for _, active := range consumeContexts {
				active.Stop()
			}
			return fmt.Errorf("start SMS consumer worker %d: %w", workerIndex, consumeErr)
		}
		consumeContexts = append(consumeContexts, consumeContext)
		go func(index int, active natsjs.ConsumeContext) {
			<-active.Closed()
			if ctx.Err() != nil {
				return
			}
			select {
			case consumeErrors <- fmt.Errorf("SMS consumer worker %d stopped unexpectedly", index):
			default:
			}
		}(workerIndex, consumeContext)
	}

	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-consumeErrors:
	}

	for _, consumeContext := range consumeContexts {
		consumeContext.Drain()
	}
	for _, consumeContext := range consumeContexts {
		select {
		case <-consumeContext.Closed():
		case <-time.After(c.config.HandlerTimeout):
			consumeContext.Stop()
		}
	}
	return runErr
}

func (c *Consumer) processMessage(parent context.Context, message natsjs.Msg) {
	metadata, metadataErr := message.Metadata()
	if metadataErr != nil {
		c.retryMessage(message, 1, fmt.Errorf("read JetStream metadata: %w", metadataErr))
		return
	}

	command, err := decodeCommand(message)
	if err != nil {
		c.deadLetter(parent, message, metadata, uuid.Nil, err)
		return
	}

	processed, err := c.processed.IsProcessed(parent, DeliverConsumerName, command.EventID)
	if err != nil {
		c.retryMessage(message, metadata.NumDelivered, err)
		return
	}
	if processed {
		c.ackMessage(parent, message, command.EventID)
		return
	}

	handlerContext, cancelHandler := context.WithTimeout(parent, c.config.HandlerTimeout)
	err = c.handler.Handle(handlerContext, command)
	cancelHandler()
	if err != nil {
		if parent.Err() != nil {
			return
		}
		if int(metadata.NumDelivered) >= c.config.MaxDeliver {
			finalizeContext, cancelFinalize := context.WithTimeout(parent, c.config.HandlerTimeout)
			finalizeErr := c.handler.HandleExhausted(finalizeContext, command, err)
			cancelFinalize()
			if finalizeErr != nil {
				c.retryMessage(message, metadata.NumDelivered, errors.Join(err, finalizeErr))
				return
			}
			c.deadLetter(parent, message, metadata, command.EventID, err)
			return
		}
		c.retryMessage(message, metadata.NumDelivered, err)
		return
	}

	if err := c.processed.MarkProcessed(parent, DeliverConsumerName, command.EventID, map[string]any{
		"subject":         message.Subject(),
		"stream_sequence": metadata.Sequence.Stream,
		"deliveries":      metadata.NumDelivered,
	}); err != nil {
		c.retryMessage(message, metadata.NumDelivered, err)
		return
	}

	c.ackMessage(parent, message, command.EventID)
}

func decodeCommand(message natsjs.Msg) (DeliverCommand, error) {
	var command DeliverCommand
	if err := json.Unmarshal(message.Data(), &command); err != nil {
		return DeliverCommand{}, fmt.Errorf("decode SMS delivery command: %w", err)
	}
	if command.EventID == uuid.Nil || command.MessageID == uuid.Nil || command.TeamID == uuid.Nil {
		return DeliverCommand{}, errors.New("SMS delivery command requires event, message, and team IDs")
	}

	headerEventID := strings.TrimSpace(message.Headers().Get("Dugble-Event-Id"))
	if headerEventID == "" {
		return DeliverCommand{}, errors.New("SMS delivery command is missing Dugble-Event-Id")
	}
	parsedHeaderEventID, err := uuid.Parse(headerEventID)
	if err != nil {
		return DeliverCommand{}, fmt.Errorf("parse Dugble-Event-Id: %w", err)
	}
	if parsedHeaderEventID != command.EventID {
		return DeliverCommand{}, errors.New("SMS delivery event ID does not match the outbox header")
	}
	return command, nil
}

func (c *Consumer) retryMessage(message natsjs.Msg, delivered uint64, cause error) {
	delay := c.retryDelay(delivered)
	if err := message.NakWithDelay(delay); err != nil {
		slog.Error("failed to negatively acknowledge SMS delivery command", "delay", delay, "error", err, "cause", cause)
		return
	}
	slog.Warn("SMS delivery command will be retried", "delivery", delivered, "delay", delay, "error", cause)
}

func (c *Consumer) retryDelay(delivered uint64) time.Duration {
	index := int(delivered)
	if index < 1 {
		index = 1
	}
	index--
	if index >= len(c.config.RetryDelays) {
		index = len(c.config.RetryDelays) - 1
	}
	return c.config.RetryDelays[index]
}

func (c *Consumer) ackMessage(parent context.Context, message natsjs.Msg, eventID uuid.UUID) {
	ackContext, cancelAck := context.WithTimeout(parent, 5*time.Second)
	defer cancelAck()
	if err := message.DoubleAck(ackContext); err != nil {
		slog.Warn("failed to confirm SMS delivery acknowledgement", "event_id", eventID, "error", err)
	}
}

func (c *Consumer) deadLetter(
	ctx context.Context,
	message natsjs.Msg,
	metadata *natsjs.MsgMetadata,
	eventID uuid.UUID,
	cause error,
) {
	headers := make(map[string]string, len(message.Headers())+4)
	for key, values := range message.Headers() {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	headers["Dugble-Original-Subject"] = message.Subject()
	headers["Dugble-Dead-Letter-Reason"] = truncateDeadLetterReason(cause)
	headers["Dugble-Delivery-Count"] = strconv.FormatUint(metadata.NumDelivered, 10)

	messageID := eventID.String() + "-dlq"
	if eventID == uuid.Nil {
		messageID = fmt.Sprintf("%s-%d-dlq", DeliverConsumerName, metadata.Sequence.Stream)
	}
	if err := c.publisher.Publish(ctx, DeliverDLQSubject, message.Data(), headers, messageID); err != nil {
		c.retryMessage(message, metadata.NumDelivered, fmt.Errorf("publish SMS command to DLQ: %w", err))
		return
	}
	if err := message.TermWithReason(truncateDeadLetterReason(cause)); err != nil {
		slog.Error("failed to terminate dead-lettered SMS command", "event_id", eventID, "error", err)
		return
	}
	slog.Error("SMS delivery command moved to DLQ", "event_id", eventID, "delivery", metadata.NumDelivered, "error", cause)
}

func truncateDeadLetterReason(err error) string {
	if err == nil {
		return "unknown SMS delivery failure"
	}
	reason := strings.TrimSpace(err.Error())
	if reason == "" {
		return "unknown SMS delivery failure"
	}
	if len(reason) <= maxDeadLetterReasonLength {
		return reason
	}
	return reason[:maxDeadLetterReasonLength]
}
