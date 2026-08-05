package systememail

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	natsjs "github.com/nats-io/nats.go/jetstream"

	jetstreammessaging "github.com/coffeyvidzro/dugble/server/internal/adapters/nats"
	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/awsses"
)

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
	processor *Processor
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
	return &Consumer{provider: client, processed: processed, processor: NewProcessor(sender), config: config}
}

func (consumer *Consumer) Run(ctx context.Context) error {
	if consumer == nil || consumer.provider == nil || consumer.processed == nil || consumer.processor == nil {
		return ErrConsumerNotConfigured
	}
	streamConsumer, err := consumer.provider.CreateOrUpdateConsumer(ctx, jetstreammessaging.JobsStreamName, natsjs.ConsumerConfig{
		Name: DeliverConsumerName, Durable: DeliverConsumerName, Description: "Durable Dugble system email jobs",
		DeliverPolicy: natsjs.DeliverAllPolicy, AckPolicy: natsjs.AckExplicitPolicy, AckWait: consumer.config.AckWait,
		MaxDeliver: consumer.config.MaxDeliver, FilterSubject: DeliverSubject, ReplayPolicy: natsjs.ReplayInstantPolicy,
		MaxAckPending: consumer.config.Concurrency * 4, MaxWaiting: consumer.config.Concurrency * 2, MaxRequestBatch: 1,
	})
	if err != nil {
		return fmt.Errorf("provision system email consumer: %w", err)
	}
	contexts := make([]natsjs.ConsumeContext, 0, consumer.config.Concurrency)
	for range consumer.config.Concurrency {
		active, err := streamConsumer.Consume(func(message natsjs.Msg) { consumer.process(ctx, message) }, natsjs.PullMaxMessages(1))
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

func (consumer *Consumer) process(parent context.Context, message natsjs.Msg) {
	metadata, err := message.Metadata()
	if err != nil {
		_ = message.Nak()
		return
	}
	var command DeliverCommand
	if err := json.Unmarshal(message.Data(), &command); err != nil || ValidateCommand(command) != nil {
		_ = message.TermWithReason("invalid system email command")
		return
	}
	processed, err := consumer.processed.IsProcessed(parent, DeliverConsumerName, command.EventID)
	if err != nil {
		_ = message.Nak()
		return
	}
	if processed {
		_ = message.Ack()
		return
	}
	ctx, cancel := context.WithTimeout(parent, consumer.config.HandlerTimeout)
	err = consumer.processor.Handle(ctx, command)
	cancel()
	if err != nil {
		if int(metadata.NumDelivered) >= consumer.config.MaxDeliver {
			_ = message.TermWithReason("system email delivery exhausted")
			slog.Error("system email delivery exhausted", "event_id", command.EventID, "error", err)
			return
		}
		_ = message.NakWithDelay(retryDelay(metadata.NumDelivered))
		return
	}
	if err := consumer.processed.MarkProcessed(parent, DeliverConsumerName, command.EventID, map[string]any{
		"subject": message.Subject(), "deliveries": metadata.NumDelivered,
	}); err != nil {
		_ = message.Nak()
		return
	}
	_ = message.Ack()
}
