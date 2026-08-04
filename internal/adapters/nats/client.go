package nats

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	natsgo "github.com/nats-io/nats.go"
	natsjs "github.com/nats-io/nats.go/jetstream"
	"github.com/newrelic/go-agent/v3/newrelic"
)

const (
	defaultClientName     = "dugble"
	defaultConnectTimeout = 5 * time.Second
	defaultReconnectWait  = 2 * time.Second
)

var ErrClientUnavailable = errors.New("JetStream client is unavailable")

type Client struct {
	connection *natsgo.Conn
	jetStream  natsjs.JetStream
	monitoring *newrelic.Application
}

func New(
	ctx context.Context,
	serverURL string,
	clientName string,
	applications ...*newrelic.Application,
) (*Client, error) {
	if ctx == nil {
		return nil, errors.New("NATS context is required")
	}
	serverURL = strings.TrimSpace(serverURL)
	if serverURL == "" {
		return nil, errors.New("NATS URL is required")
	}
	clientName = strings.TrimSpace(clientName)
	if clientName == "" {
		clientName = defaultClientName
	}

	var monitoring *newrelic.Application
	if len(applications) > 0 {
		monitoring = applications[0]
	}
	connection, err := natsgo.Connect(serverURL, connectionOptions(clientName)...)
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

func (client *Client) Close() error {
	if client == nil || client.connection == nil || client.connection.IsClosed() {
		return nil
	}
	if err := client.connection.Drain(); err != nil {
		client.connection.Close()
		return fmt.Errorf("drain NATS connection: %w", err)
	}
	return nil
}

func connectionOptions(clientName string) []natsgo.Option {
	return []natsgo.Option{
		natsgo.Name(clientName),
		natsgo.Timeout(defaultConnectTimeout),
		natsgo.MaxReconnects(-1),
		natsgo.ReconnectWait(defaultReconnectWait),
		natsgo.DisconnectErrHandler(func(_ *natsgo.Conn, err error) {
			if err != nil {
				slog.Warn("NATS disconnected", "error", err)
			}
		}),
		natsgo.ReconnectHandler(func(connection *natsgo.Conn) {
			slog.Info("NATS reconnected", "server", connection.ConnectedUrlRedacted())
		}),
		natsgo.ClosedHandler(func(connection *natsgo.Conn) {
			if err := connection.LastError(); err != nil {
				slog.Error("NATS connection closed", "error", err)
			}
		}),
	}
}
