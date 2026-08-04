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

	jetstreammessaging "github.com/coffeyvidzro/dugble/server/internal/messaging/jetstream"
	"github.com/coffeyvidzro/dugble/server/internal/modules/emailtenant"
)

const (
	ConsumerName = "dugble-email-tenant-provision-v1"
	DLQSubject   = "dugble.dlq.email.tenant.provision.v1"
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

type commandHandler interface {
	Handle(context.Context, emailtenant.ProvisionCommand) error
	HandleExhausted(context.Context, emailtenant.ProvisionCommand, error) error
}

type Config struct {
	Concurrency    int
	AckWait        time.Duration
	HandlerTimeout time.Duration
	MaxDeliver     int
	RetryBackOff   []time.Duration
}

var defaultRetryBackOff = []time.Duration{time.Second, 5 * time.Second, 30 * time.Second, 2 * time.Minute, 5 * time.Minute, 15 * time.Minute}

func DefaultRetryBackOff() []time.Duration {
	return append([]time.Duration(nil), defaultRetryBackOff...)
}

type Consumer struct {
	provider  consumerProvider
	publisher messagePublisher
	processed processedEventStore
	handler   commandHandler
	config    Config
}

func NewConsumer(client *jetstreammessaging.Client, processed processedEventStore, handler commandHandler, config Config) *Consumer {
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
	return &Consumer{provider: client, publisher: client, processed: processed, handler: handler, config: config}
}

func (c *Consumer) Run(ctx context.Context) error {
	if c == nil || c.provider == nil || c.publisher == nil || c.processed == nil || c.handler == nil {
		return errors.New("email tenant provisioning consumer is not fully configured")
	}
	consumer, err := c.provider.CreateOrUpdateConsumer(ctx, jetstreammessaging.JobsStreamName, natsjs.ConsumerConfig{
		Name: ConsumerName, Durable: ConsumerName, Description: "Durable SES tenant provisioning jobs",
		DeliverPolicy: natsjs.DeliverAllPolicy, AckPolicy: natsjs.AckExplicitPolicy, AckWait: c.config.AckWait,
		MaxDeliver: c.config.MaxDeliver, BackOff: c.config.RetryBackOff,
		FilterSubject: emailtenant.ProvisionSubject, ReplayPolicy: natsjs.ReplayInstantPolicy,
		MaxAckPending: c.config.Concurrency * 4, MaxWaiting: c.config.Concurrency * 2, MaxRequestBatch: 1,
	})
	if err != nil {
		return fmt.Errorf("provision email tenant consumer: %w", err)
	}
	contexts := make([]natsjs.ConsumeContext, 0, c.config.Concurrency)
	for worker := range c.config.Concurrency {
		active, consumeErr := consumer.Consume(func(message natsjs.Msg) { c.process(ctx, message) }, natsjs.PullMaxMessages(1))
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

func (c *Consumer) process(parent context.Context, message natsjs.Msg) {
	metadata, err := message.Metadata()
	if err != nil {
		_ = message.Nak()
		return
	}
	command, err := decodeCommand(message)
	if err != nil {
		c.deadLetter(parent, message, metadata, uuid.Nil, err)
		return
	}
	processed, err := c.processed.IsProcessed(parent, ConsumerName, command.EventID)
	if err != nil {
		_ = message.Nak()
		return
	}
	if processed {
		_ = message.Ack()
		return
	}

	handlerCtx, cancel := context.WithTimeout(parent, c.config.HandlerTimeout)
	err = c.handler.Handle(handlerCtx, command)
	cancel()
	if err != nil {
		if parent.Err() != nil {
			return
		}
		if int(metadata.NumDelivered) >= c.config.MaxDeliver {
			finalizeCtx, finalizeCancel := context.WithTimeout(parent, c.config.HandlerTimeout)
			finalizeErr := c.handler.HandleExhausted(finalizeCtx, command, err)
			finalizeCancel()
			if finalizeErr != nil {
				_ = message.NakWithDelay(time.Minute)
				return
			}
			c.deadLetter(parent, message, metadata, command.EventID, err)
			return
		}
		_ = message.NakWithDelay(c.retryDelay(metadata.NumDelivered))
		return
	}

	if err := c.processed.MarkProcessed(parent, ConsumerName, command.EventID, map[string]any{
		"subject": message.Subject(), "stream_sequence": metadata.Sequence.Stream, "deliveries": metadata.NumDelivered,
	}); err != nil {
		_ = message.Nak()
		return
	}
	_ = message.Ack()
}

func decodeCommand(message natsjs.Msg) (emailtenant.ProvisionCommand, error) {
	var command emailtenant.ProvisionCommand
	if err := json.Unmarshal(message.Data(), &command); err != nil {
		return emailtenant.ProvisionCommand{}, fmt.Errorf("decode email tenant provisioning command: %w", err)
	}
	if command.EventID == uuid.Nil || command.TenantID == uuid.Nil || command.TeamID == uuid.Nil || command.SchemaVersion != 1 {
		return emailtenant.ProvisionCommand{}, errors.New("invalid email tenant provisioning command")
	}
	headerID, err := uuid.Parse(strings.TrimSpace(message.Headers().Get("Dugble-Event-Id")))
	if err != nil || headerID != command.EventID {
		return emailtenant.ProvisionCommand{}, errors.New("email tenant event ID does not match outbox header")
	}
	return command, nil
}

func (c *Consumer) retryDelay(delivered uint64) time.Duration {
	delays := c.config.RetryBackOff
	index := int(delivered) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(delays) {
		index = len(delays) - 1
	}
	return delays[index]
}

func normalizeRetryBackOff(delays []time.Duration, maxDeliver int) []time.Duration {
	if maxDeliver <= 0 {
		return nil
	}
	result := make([]time.Duration, maxDeliver)
	for index := range result {
		source := index
		if source >= len(delays) {
			source = len(delays) - 1
		}
		result[index] = delays[source]
	}
	return result
}

func (c *Consumer) deadLetter(ctx context.Context, message natsjs.Msg, metadata *natsjs.MsgMetadata, eventID uuid.UUID, cause error) {
	headers := map[string]string{
		"Dugble-Original-Subject":   message.Subject(),
		"Dugble-Dead-Letter-Reason": truncateReason(cause),
		"Dugble-Delivery-Count":     strconv.FormatUint(metadata.NumDelivered, 10),
	}
	messageID := eventID.String() + "-dlq"
	if eventID == uuid.Nil {
		messageID = fmt.Sprintf("%s-%d-dlq", ConsumerName, metadata.Sequence.Stream)
	}
	if err := c.publisher.Publish(ctx, DLQSubject, message.Data(), headers, messageID); err != nil {
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
