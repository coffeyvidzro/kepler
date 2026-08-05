package arkesel

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
	productionBaseURL    = "https://sms.arkesel.com"
	defaultClientTimeout = 30 * time.Second
	maxErrorResponseBody = 32 << 10
)

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewClient(config config.ProviderConfig) *Client {
	return NewClientWithHTTP(config, nil)
}

func NewClientWithHTTP(config config.ProviderConfig, httpClient *http.Client) *Client {
	return newClient(productionBaseURL, config, httpClient)
}

func newClient(baseURL string, config config.ProviderConfig, httpClient *http.Client) *Client {
	httpClient = platformhttp.ForFixedEndpoint(baseURL, httpClient, defaultClientTimeout)
	return &Client{
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:     strings.TrimSpace(config.APIKey),
		httpClient: httpClient,
	}
}

func (client *Client) do(ctx context.Context, method, path string, payload, result any) error {
	if client == nil || client.httpClient == nil {
		return ErrClientUnavailable
	}
	if ctx == nil {
		return fmt.Errorf("Arkesel request context is required")
	}
	if client.apiKey == "" {
		return fmt.Errorf("Arkesel API key is required")
	}
	var body io.Reader = http.NoBody
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode Arkesel request: %w", err)
		}
		body = bytes.NewReader(data)
	}
	endpoint := client.baseURL + "/" + strings.TrimLeft(path, "/")
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fmt.Errorf("create Arkesel request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("api-key", client.apiKey)
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("send Arkesel request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		responseBody, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorResponseBody))
		return &APIError{StatusCode: response.StatusCode, Body: strings.TrimSpace(string(responseBody))}
	}
	if result == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(result); err != nil {
		return fmt.Errorf("decode Arkesel response: %w", err)
	}
	return nil
}
