package systememail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	natsjs "github.com/nats-io/nats.go/jetstream"

	jetstreammessaging "github.com/coffeyvidzro/dugble/server/internal/messaging/jetstream"
	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"
)

const DeliverConsumerName = "dugble-system-email-delivery-v1"

type processedEventStore interface {
	IsProcessed(context.Context, string, uuid.UUID) (bool, error)
	MarkProcessed(context.Context, string, uuid.UUID, map[string]any) error
}

type consumerProvider interface {
	CreateOrUpdateConsumer(context.Context, string, natsjs.ConsumerConfig) (natsjs.Consumer, error)
}

type ConsumerConfig struct {
	Concurrency    int
	AckWait        time.Duration
	HandlerTimeout time.Duration
	MaxDeliver     int
}

type Consumer struct {
	provider  consumerProvider
	processed processedEventStore
	sender    platformemail.Sender
	config    ConsumerConfig
}

func NewConsumer(client *jetstreammessaging.Client, processed processedEventStore, sender platformemail.Sender, config ConsumerConfig) *Consumer {
	if config.Concurrency <= 0 {
		config.Concurrency = 3
	}
	if config.AckWait <= 0 {
		config.AckWait = time.Minute
	}
	if config.HandlerTimeout <= 0 {
		config.HandlerTimeout = 30 * time.Second
	}
	if config.MaxDeliver <= 0 {
		config.MaxDeliver = 6
	}
	return &Consumer{provider: client, processed: processed, sender: sender, config: config}
}

func (c *Consumer) Run(ctx context.Context) error {
	if c == nil || c.provider == nil || c.processed == nil || c.sender == nil {
		return errors.New("system email consumer is not fully configured")
	}
	consumer, err := c.provider.CreateOrUpdateConsumer(ctx, jetstreammessaging.JobsStreamName, natsjs.ConsumerConfig{
		Name: DeliverConsumerName, Durable: DeliverConsumerName, Description: "Durable Dugble system email jobs",
		DeliverPolicy: natsjs.DeliverAllPolicy, AckPolicy: natsjs.AckExplicitPolicy, AckWait: c.config.AckWait,
		MaxDeliver: c.config.MaxDeliver, FilterSubject: DeliverSubject, ReplayPolicy: natsjs.ReplayInstantPolicy,
		MaxAckPending: c.config.Concurrency * 4, MaxWaiting: c.config.Concurrency * 2, MaxRequestBatch: 1,
	})
	if err != nil {
		return fmt.Errorf("provision system email consumer: %w", err)
	}
	contexts := make([]natsjs.ConsumeContext, 0, c.config.Concurrency)
	for range c.config.Concurrency {
		active, err := consumer.Consume(func(message natsjs.Msg) { c.process(ctx, message) }, natsjs.PullMaxMessages(1))
		if err != nil {
			for _, item := range contexts {
				item.Stop()
			}
			return fmt.Errorf("start system email consumer: %w", err)
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
	var command DeliverCommand
	if err := json.Unmarshal(message.Data(), &command); err != nil || command.EventID == uuid.Nil || command.SchemaVersion != 1 {
		_ = message.TermWithReason("invalid system email command")
		return
	}
	processed, err := c.processed.IsProcessed(parent, DeliverConsumerName, command.EventID)
	if err != nil {
		_ = message.Nak()
		return
	}
	if processed {
		_ = message.Ack()
		return
	}
	ctx, cancel := context.WithTimeout(parent, c.config.HandlerTimeout)
	_, err = c.sender.Send(ctx, command.Message)
	cancel()
	if err != nil {
		if int(metadata.NumDelivered) >= c.config.MaxDeliver {
			_ = message.TermWithReason("system email delivery exhausted")
			slog.Error("system email delivery exhausted", "event_id", command.EventID, "error", err)
			return
		}
		_ = message.NakWithDelay(time.Duration(metadata.NumDelivered) * time.Second)
		return
	}
	if err := c.processed.MarkProcessed(parent, DeliverConsumerName, command.EventID, map[string]any{"subject": message.Subject(), "deliveries": metadata.NumDelivered}); err != nil {
		_ = message.Nak()
		return
	}
	_ = message.Ack()
}
