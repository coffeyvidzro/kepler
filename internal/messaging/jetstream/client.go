package jetstream

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	natsjs "github.com/nats-io/nats.go/jetstream"
	"github.com/newrelic/go-agent/v3/integrations/nrnats"
	"github.com/newrelic/go-agent/v3/newrelic"
)

type Client struct {
	connection *nats.Conn
	jetStream  natsjs.JetStream
	monitoring *newrelic.Application
}

func New(
	ctx context.Context,
	url string,
	name string,
	applications ...*newrelic.Application,
) (*Client, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return nil, fmt.Errorf("NATS URL is required")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "dugble"
	}

	var monitoring *newrelic.Application
	if len(applications) > 0 {
		monitoring = applications[0]
	}

	connection, err := nats.Connect(
		url,
		nats.Name(name),
		nats.Timeout(5*time.Second),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
		nats.DisconnectErrHandler(func(_ *nats.Conn, disconnectErr error) {
			if disconnectErr != nil {
				slog.Warn("NATS disconnected", "error", disconnectErr)
			}
		}),
		nats.ReconnectHandler(func(connection *nats.Conn) {
			slog.Info("NATS reconnected", "server", connection.ConnectedUrlRedacted())
		}),
		nats.ClosedHandler(func(connection *nats.Conn) {
			if closeErr := connection.LastError(); closeErr != nil {
				slog.Error("NATS connection closed", "error", closeErr)
			}
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to NATS: %w", err)
	}

	jetStream, err := natsjs.New(connection)
	if err != nil {
		connection.Close()
		return nil, fmt.Errorf("initialize JetStream: %w", err)
	}

	if _, err := jetStream.AccountInfo(ctx); err != nil {
		connection.Close()
		return nil, fmt.Errorf("verify JetStream account: %w", err)
	}

	return &Client{
		connection: connection,
		jetStream:  jetStream,
		monitoring: monitoring,
	}, nil
}

func (c *Client) Provision(ctx context.Context, limits StreamLimits) error {
	if c == nil || c.jetStream == nil {
		return fmt.Errorf("JetStream client is not configured")
	}

	for _, config := range StreamConfigs(limits) {
		if _, err := c.jetStream.CreateOrUpdateStream(ctx, config); err != nil {
			return fmt.Errorf("provision JetStream stream %s: %w", config.Name, err)
		}
	}

	return nil
}

func (c *Client) CreateOrUpdateConsumer(
	ctx context.Context,
	stream string,
	config natsjs.ConsumerConfig,
) (natsjs.Consumer, error) {
	if c == nil || c.jetStream == nil {
		return nil, fmt.Errorf("JetStream client is not configured")
	}
	stream = strings.TrimSpace(stream)
	if stream == "" {
		return nil, fmt.Errorf("JetStream stream name is required")
	}

	consumer, err := c.jetStream.CreateOrUpdateConsumer(ctx, stream, config)
	if err != nil {
		return nil, fmt.Errorf("create or update consumer %s on %s: %w", config.Durable, stream, err)
	}
	return consumer, nil
}

func (c *Client) Publish(
	ctx context.Context,
	subject string,
	payload []byte,
	headers map[string]string,
	messageID string,
) error {
	if c == nil || c.jetStream == nil || c.connection == nil {
		return fmt.Errorf("JetStream client is not configured")
	}

	message := &nats.Msg{
		Subject: strings.TrimSpace(subject),
		Data:    payload,
		Header:  nats.Header{},
	}
	for key, value := range headers {
		message.Header.Set(key, value)
	}

	txn := newrelic.FromContext(ctx)
	ownsTransaction := false
	if txn == nil && c.monitoring != nil {
		txn = c.monitoring.StartTransaction("NATS publish " + message.Subject)
		ctx = newrelic.NewContext(ctx, txn)
		ownsTransaction = true
	}
	if ownsTransaction {
		defer txn.End()
	}
	if txn != nil {
		txn.AddAttribute("messaging.system", "nats")
		txn.AddAttribute("messaging.destination", message.Subject)
		txn.InsertDistributedTraceHeaders(http.Header(message.Header))
		defer nrnats.StartPublishSegment(txn, c.connection, message.Subject).End()
	}

	options := make([]natsjs.PublishOpt, 0, 1)
	if messageID = strings.TrimSpace(messageID); messageID != "" {
		options = append(options, natsjs.WithMsgID(messageID))
	}

	if _, err := c.jetStream.PublishMsg(ctx, message, options...); err != nil {
		wrappedErr := fmt.Errorf("publish JetStream message to %s: %w", message.Subject, err)
		if txn != nil {
			txn.NoticeError(wrappedErr)
		}
		return wrappedErr
	}

	return nil
}

func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.connection == nil || c.jetStream == nil || !c.connection.IsConnected() {
		return fmt.Errorf("JetStream client is not connected")
	}
	if _, err := c.jetStream.AccountInfo(ctx); err != nil {
		return fmt.Errorf("read JetStream account info: %w", err)
	}
	return nil
}

func (c *Client) Close() error {
	if c == nil || c.connection == nil || c.connection.IsClosed() {
		return nil
	}

	if err := c.connection.Drain(); err != nil {
		c.connection.Close()
		return fmt.Errorf("drain NATS connection: %w", err)
	}

	return nil
}
