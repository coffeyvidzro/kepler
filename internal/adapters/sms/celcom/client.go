package celcom

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/coffeyvidzro/dugble/server/internal/config"
	platformhttp "github.com/coffeyvidzro/dugble/server/internal/platform/httpclient"
)

const (
	productionBaseURL    = "https://isms.celcomafrica.com"
	defaultClientTimeout = 30 * time.Second
	maxResponseBodyBytes = 1 << 20
)

type Client struct {
	baseURL    string
	apiKey     string
	partnerID  string
	httpClient *http.Client
}

func NewClient(config config.CelcomConfig) *Client { return NewClientWithHTTP(config, nil) }

func NewClientWithHTTP(config config.CelcomConfig, httpClient *http.Client) *Client {
	return newClient(productionBaseURL, config, httpClient)
}

func newClient(baseURL string, config config.CelcomConfig, httpClient *http.Client) *Client {
	httpClient = platformhttp.ForFixedEndpoint(baseURL, httpClient, defaultClientTimeout)
	return &Client{
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:     strings.TrimSpace(config.APIKey),
		partnerID:  strings.TrimSpace(config.PartnerID),
		httpClient: httpClient,
	}
}

func (client *Client) do(ctx context.Context, path string, payload, result any) error {
	if client == nil || client.httpClient == nil {
		return ErrClientUnavailable
	}
	if ctx == nil {
		return fmt.Errorf("Celcom request context is required")
	}
	if client.apiKey == "" {
		return fmt.Errorf("Celcom API key is required")
	}
	if client.partnerID == "" {
		return fmt.Errorf("Celcom partner ID is required")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode Celcom request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.baseURL+"/"+strings.TrimLeft(path, "/"), bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create Celcom request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("send Celcom request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBodyBytes))
	if err != nil {
		return fmt.Errorf("read Celcom response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return &APIError{HTTPStatus: response.StatusCode, Body: strings.TrimSpace(string(responseBody))}
	}
	if result == nil || len(bytes.TrimSpace(responseBody)) == 0 {
		return nil
	}
	if err := json.Unmarshal(responseBody, result); err != nil {
		return fmt.Errorf("decode Celcom response: %w", err)
	}
	return nil
}
