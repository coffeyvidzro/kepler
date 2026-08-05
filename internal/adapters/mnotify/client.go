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

	platformhttp "github.com/coffeyvidzro/dugble/server/internal/platform/httpclient"
)

const (
	productionBaseURL    = "https://api.mnotify.com"
	defaultClientTimeout = 30 * time.Second
	maxResponseBodyBytes = 1 << 20
)

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewClient(apiKey string) *Client {
	return newClient(productionBaseURL, apiKey, nil)
}

// NewClientWithHTTP keeps the production endpoint fixed while allowing tests
// and instrumented deployments to provide their own HTTP transport.
func NewClientWithHTTP(apiKey string, httpClient *http.Client) *Client {
	return newClient(productionBaseURL, apiKey, httpClient)
}

func newClient(baseURL, apiKey string, httpClient *http.Client) *Client {
	httpClient = platformhttp.ForFixedEndpoint(baseURL, httpClient, defaultClientTimeout)
	return &Client{
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:     strings.TrimSpace(apiKey),
		httpClient: httpClient,
	}
}

func (client *Client) Get(ctx context.Context, path string, result any) error {
	return client.do(ctx, http.MethodGet, path, nil, result)
}

func (client *Client) Post(ctx context.Context, path string, payload, result any) error {
	return client.do(ctx, http.MethodPost, path, payload, result)
}

func (client *Client) do(ctx context.Context, method, path string, payload, result any) error {
	if client == nil || client.httpClient == nil {
		return ErrClientUnavailable
	}
	if ctx == nil {
		return fmt.Errorf("mNotify request context is required")
	}
	if client.apiKey == "" {
		return fmt.Errorf("mNotify API key is required")
	}

	requestURL, err := buildRequestURL(client.baseURL, path, client.apiKey)
	if err != nil {
		return fmt.Errorf("create mNotify request URL: %w", err)
	}

	var body io.Reader = http.NoBody
	if payload != nil {
		data, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return fmt.Errorf("encode mNotify request: %w", marshalErr)
		}
		body = bytes.NewReader(data)
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

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBodyBytes+1))
	if err != nil {
		return fmt.Errorf("read mNotify response: %w", err)
	}
	if len(responseBody) > maxResponseBodyBytes {
		return fmt.Errorf("%w: response body exceeds %d bytes", ErrInvalidResponse, maxResponseBodyBytes)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return apiError(response.StatusCode, responseBody)
	}
	if result == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	if len(bytes.TrimSpace(responseBody)) == 0 {
		return fmt.Errorf("%w: response body is empty", ErrInvalidResponse)
	}
	if err := json.Unmarshal(responseBody, result); err != nil {
		return fmt.Errorf("decode mNotify response: %w", err)
	}
	return nil
}

func buildRequestURL(baseURL, path, apiKey string) (string, error) {
	requestURL, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/" + strings.TrimLeft(strings.TrimSpace(path), "/"))
	if err != nil {
		return "", err
	}
	if requestURL.Scheme == "" || requestURL.Host == "" {
		return "", fmt.Errorf("mNotify base URL must include scheme and host")
	}
	query := requestURL.Query()
	query.Set("key", strings.TrimSpace(apiKey))
	requestURL.RawQuery = query.Encode()
	return requestURL.String(), nil
}

func apiError(statusCode int, body []byte) error {
	response := Response{}
	if err := json.Unmarshal(body, &response); err == nil {
		return &APIError{
			StatusCode: statusCode,
			Status:     strings.TrimSpace(response.Status),
			Code:       response.Code,
			Message:    strings.TrimSpace(response.Message),
			Body:       strings.TrimSpace(string(body)),
		}
	}
	return &APIError{StatusCode: statusCode, Body: strings.TrimSpace(string(body))}
}
