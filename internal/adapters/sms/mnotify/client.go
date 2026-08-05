package mnotify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coffeyvidzro/dugble/server/internal/config"
)

const (
	defaultBaseURL       = "https://api.mnotify.com"
	defaultClientTimeout = 30 * time.Second
	maxResponseBodyBytes = 1 << 20
)

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewClient(config config.ProviderConfig) *Client { return NewClientWithHTTP(config, nil) }

func NewClientWithHTTP(config config.ProviderConfig, httpClient *http.Client) *Client {
	baseURL := strings.TrimSpace(config.BaseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultClientTimeout}
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), apiKey: strings.TrimSpace(config.APIKey), httpClient: httpClient}
}

func (client *Client) do(ctx context.Context, method, path string, payload, result any) error {
	if client == nil || client.httpClient == nil {
		return ErrClientUnavailable
	}
	if ctx == nil {
		return fmt.Errorf("mNotify request context is required")
	}
	if client.baseURL == "" {
		return fmt.Errorf("mNotify base URL is required")
	}
	if client.apiKey == "" {
		return fmt.Errorf("mNotify API key is required")
	}
	var body io.Reader = http.NoBody
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode mNotify request: %w", err)
		}
		body = bytes.NewReader(data)
	}
	requestURL, err := buildRequestURL(client.baseURL, path, client.apiKey)
	if err != nil {
		return fmt.Errorf("create mNotify request URL: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return fmt.Errorf("create mNotify request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("send mNotify request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBodyBytes))
	if err != nil {
		return fmt.Errorf("read mNotify response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return &APIError{StatusCode: response.StatusCode, Body: strings.TrimSpace(string(responseBody))}
	}
	if result == nil || response.StatusCode == http.StatusNoContent || len(bytes.TrimSpace(responseBody)) == 0 {
		return nil
	}
	if err := json.Unmarshal(responseBody, result); err != nil {
		return fmt.Errorf("decode mNotify response: %w", err)
	}
	return nil
}

func buildRequestURL(baseURL, path, apiKey string) (string, error) {
	requestURL, err := url.Parse(strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/"))
	if err != nil {
		return "", err
	}
	if requestURL.Scheme == "" || requestURL.Host == "" {
		return "", fmt.Errorf("mNotify base URL must include scheme and host")
	}
	query := requestURL.Query()
	query.Set("key", apiKey)
	requestURL.RawQuery = query.Encode()
	return requestURL.String(), nil
}
