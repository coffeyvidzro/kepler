package emaildelivery

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
	DeliverConsumerName = "dugble-email-delivery-v1"
	DeliverDLQSubject   = "dugble.dlq.email.send.v1"
)

type processedEventStore interface {
	IsProcessed(context.Context, string, uuid.UUID) (bool, error)
	MarkProcessed(context.Context, string, uuid.UUID, map[string]any) error
}

type deliveryHandler interface {
	Handle(context.Context, DeliverCommand) error
	HandleExhausted(context.Context, DeliverCommand, error) error
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
	RetryPolicy    RetryPolicy
}

type Consumer struct {
	provider  consumerProvider
	publisher messagePublisher
	processed processedEventStore
	handler   deliveryHandler
	config    ConsumerConfig
}

func NewConsumer(client *jetstreammessaging.Client, processed processedEventStore, handler deliveryHandler, config ConsumerConfig) *Consumer {
	return &Consumer{
		provider:  client,
		publisher: client,
		processed: processed,
		handler:   handler,
		config:    normalizeConsumerConfig(config),
	}
}

func normalizeConsumerConfig(config ConsumerConfig) ConsumerConfig {
	if config.Concurrency <= 0 {
		config.Concurrency = 5
	}
	if config.AckWait <= 0 {
		config.AckWait = 2 * time.Minute
	}
	if config.HandlerTimeout <= 0 {
		config.HandlerTimeout = 45 * time.Second
	}
	if len(config.RetryPolicy.Delays) == 0 {
		config.RetryPolicy = DefaultRetryPolicy()
	}
	if config.MaxDeliver <= 0 {
		config.MaxDeliver = len(config.RetryPolicy.Delays)
	}
	return config
}

func (c *Consumer) Run(ctx context.Context) error {
	if c == nil || c.provider == nil || c.publisher == nil || c.processed == nil || c.handler == nil {
		return errors.New("email delivery consumer is not fully configured")
	}
	consumer, err := c.provider.CreateOrUpdateConsumer(ctx, jetstreammessaging.JobsStreamName, natsjs.ConsumerConfig{
		Name:            DeliverConsumerName,
		Durable:         DeliverConsumerName,
		Description:     "Durable email delivery commands",
		DeliverPolicy:   natsjs.DeliverAllPolicy,
		AckPolicy:       natsjs.AckExplicitPolicy,
		AckWait:         c.config.AckWait,
		MaxDeliver:      c.config.MaxDeliver,
		BackOff:         append([]time.Duration(nil), c.config.RetryPolicy.Delays...),
		FilterSubject:   DeliverSubject,
		ReplayPolicy:    natsjs.ReplayInstantPolicy,
		MaxAckPending:   max(c.config.Concurrency*4, c.config.Concurrency),
		MaxWaiting:      max(c.config.Concurrency*2, c.config.Concurrency),
		MaxRequestBatch: 1,
	})
	if err != nil {
		return fmt.Errorf("provision email delivery consumer: %w", err)
	}

	contexts := make([]natsjs.ConsumeContext, 0, c.config.Concurrency)
	errorsChannel := make(chan error, c.config.Concurrency)
	for workerIndex := range c.config.Concurrency {
		consumeContext, consumeErr := consumer.Consume(
			func(message natsjs.Msg) { c.processMessage(ctx, message) },
			natsjs.PullMaxMessages(1),
			natsjs.ConsumeErrHandler(func(_ natsjs.ConsumeContext, consumeErr error) {
				if consumeErr != nil {
					slog.Warn("email consumer encountered a JetStream error", "worker", workerIndex, "error", consumeErr)
				}
			}),
		)
		if consumeErr != nil {
			for _, active := range contexts {
				active.Stop()
			}
			return fmt.Errorf("start email consumer worker %d: %w", workerIndex, consumeErr)
		}
		contexts = append(contexts, consumeContext)
		go func(index int, active natsjs.ConsumeContext) {
			<-active.Closed()
			if ctx.Err() == nil {
				select {
				case errorsChannel <- fmt.Errorf("email consumer worker %d stopped unexpectedly", index):
				default:
				}
			}
		}(workerIndex, consumeContext)
	}

	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-errorsChannel:
	}
	for _, active := range contexts {
		active.Drain()
	}
	for _, active := range contexts {
		select {
		case <-active.Closed():
		case <-time.After(c.config.HandlerTimeout):
			active.Stop()
		}
	}
	return runErr
}

func (c *Consumer) processMessage(parent context.Context, message natsjs.Msg) {
	metadata, err := message.Metadata()
	if err != nil {
		c.retry(message, 1, err)
		return
	}
	command, err := decodeCommand(message)
	if err != nil {
		c.deadLetter(parent, message, metadata, uuid.Nil, err)
		return
	}
	processed, err := c.processed.IsProcessed(parent, DeliverConsumerName, command.EventID)
	if err != nil {
		c.retry(message, metadata.NumDelivered, err)
		return
	}
	if processed {
		c.ack(parent, message, command.EventID)
		return
	}

	handlerContext, cancel := context.WithTimeout(parent, c.config.HandlerTimeout)
	err = c.handler.Handle(handlerContext, command)
	cancel()
	if err != nil {
		if parent.Err() != nil {
			return
		}
		if int(metadata.NumDelivered) >= c.config.MaxDeliver {
			finalizeContext, finalizeCancel := context.WithTimeout(parent, c.config.HandlerTimeout)
			finalizeErr := c.handler.HandleExhausted(finalizeContext, command, err)
			finalizeCancel()
			if finalizeErr != nil {
				c.retry(message, metadata.NumDelivered, errors.Join(err, finalizeErr))
				return
			}
			c.deadLetter(parent, message, metadata, command.EventID, err)
			return
		}
		c.retry(message, metadata.NumDelivered, err)
		return
	}

	if err := c.processed.MarkProcessed(parent, DeliverConsumerName, command.EventID, map[string]any{
		"subject": message.Subject(), "stream_sequence": metadata.Sequence.Stream, "deliveries": metadata.NumDelivered,
	}); err != nil {
		c.retry(message, metadata.NumDelivered, err)
		return
	}
	c.ack(parent, message, command.EventID)
}

func decodeCommand(message natsjs.Msg) (DeliverCommand, error) {
	var command DeliverCommand
	if err := json.Unmarshal(message.Data(), &command); err != nil {
		return DeliverCommand{}, fmt.Errorf("decode email delivery command: %w", err)
	}
	if command.EventID == uuid.Nil || command.MessageID == uuid.Nil || command.TeamID == uuid.Nil || command.SchemaVersion != 1 {
		return DeliverCommand{}, errors.New("invalid email delivery command")
	}
	headerEventID := strings.TrimSpace(message.Headers().Get("Dugble-Event-Id"))
	parsedHeaderEventID, err := uuid.Parse(headerEventID)
	if err != nil || parsedHeaderEventID != command.EventID {
		return DeliverCommand{}, errors.New("email delivery event ID does not match the outbox header")
	}
	return command, nil
}

func (c *Consumer) retry(message natsjs.Msg, delivered uint64, cause error) {
	delay := c.config.RetryPolicy.Delay(delivered)
	if err := message.NakWithDelay(delay); err != nil {
		slog.Error("failed to negatively acknowledge email delivery command", "error", err, "cause", cause)
		return
	}
	slog.Warn("email delivery command will be retried", "delivery", delivered, "delay", delay, "error", cause)
}

func (c *Consumer) ack(parent context.Context, message natsjs.Msg, eventID uuid.UUID) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	if err := message.DoubleAck(ctx); err != nil {
		slog.Warn("failed to confirm email delivery acknowledgement", "event_id", eventID, "error", err)
	}
}

func (c *Consumer) deadLetter(ctx context.Context, message natsjs.Msg, metadata *natsjs.MsgMetadata, eventID uuid.UUID, cause error) {
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
		c.retry(message, metadata.NumDelivered, fmt.Errorf("publish email command to DLQ: %w", err))
		return
	}
	if err := message.TermWithReason(truncateDeadLetterReason(cause)); err != nil {
		slog.Error("failed to terminate dead-lettered email command", "event_id", eventID, "error", err)
		return
	}
	slog.Error("email delivery command moved to DLQ", "event_id", eventID, "delivery", metadata.NumDelivered, "error", cause)
}

func truncateDeadLetterReason(err error) string {
	if err == nil {
		return "unknown email delivery failure"
	}
	reason := strings.TrimSpace(err.Error())
	if reason == "" {
		return "unknown email delivery failure"
	}
	if len(reason) > 512 {
		return reason[:512]
	}
	return reason
}
