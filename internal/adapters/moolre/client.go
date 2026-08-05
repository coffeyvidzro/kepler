package moolre

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
)

const (
	DefaultBaseURL       = "https://api.moolre.com"
	defaultClientTimeout = 30 * time.Second
	maxResponseBodyBytes = 1 << 20
)

type Client struct {
	baseURL    string
	vasKey     string
	httpClient *http.Client
}

func NewClient(baseURL, vasKey string) *Client {
	return NewClientWithHTTP(baseURL, vasKey, nil)
}

func NewClientWithHTTP(baseURL, vasKey string, httpClient *http.Client) *Client {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultClientTimeout}
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		vasKey:     strings.TrimSpace(vasKey),
		httpClient: httpClient,
	}
}

func (client *Client) Post(ctx context.Context, path string, payload, result any) error {
	if client == nil || client.httpClient == nil {
		return ErrClientUnavailable
	}
	if ctx == nil {
		return fmt.Errorf("Moolre request context is required")
	}
	if client.vasKey == "" {
		return fmt.Errorf("Moolre VAS key is required")
	}

	requestURL, err := buildRequestURL(client.baseURL, path)
	if err != nil {
		return fmt.Errorf("create Moolre request URL: %w", err)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode Moolre request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create Moolre request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-API-VASKEY", client.vasKey)

	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("send Moolre request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBodyBytes+1))
	if err != nil {
		return fmt.Errorf("read Moolre response: %w", err)
	}
	if len(body) > maxResponseBodyBytes {
		return fmt.Errorf("%w: response body exceeds %d bytes", ErrInvalidResponse, maxResponseBodyBytes)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return apiError(response.StatusCode, body)
	}
	if result == nil {
		return nil
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return fmt.Errorf("%w: response body is empty", ErrInvalidResponse)
	}
	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("decode Moolre response: %w", err)
	}
	return nil
}

func buildRequestURL(baseURL, path string) (string, error) {
	requestURL, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/" + strings.TrimLeft(strings.TrimSpace(path), "/"))
	if err != nil {
		return "", err
	}
	if requestURL.Scheme == "" || requestURL.Host == "" {
		return "", fmt.Errorf("Moolre base URL must include scheme and host")
	}
	return requestURL.String(), nil
}

func apiError(statusCode int, body []byte) error {
	response := Envelope[json.RawMessage]{}
	if err := json.Unmarshal(body, &response); err == nil {
		return &APIError{
			StatusCode: statusCode,
			Status:     response.Status,
			Code:       strings.TrimSpace(response.Code),
			Message:    response.Message.String(),
			Body:       strings.TrimSpace(string(body)),
		}
	}
	return &APIError{StatusCode: statusCode, Body: strings.TrimSpace(string(body))}
}
