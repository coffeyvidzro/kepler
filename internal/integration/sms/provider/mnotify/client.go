package mnotify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	maxResponseBodyBytes = 1 << 20 // 1 MiB
)

type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// APIError represents a definitive mNotify rejection or a non-2xx response.
type APIError struct {
	StatusCode int
	Status     string
	Code       ResponseCode
	Message    string
	Body       string
	Definitive bool
}

// SafeToFallback is retained for the delivery worker's existing provider-error
// classification contract. SMS routing no longer performs provider fallback.
func (e *APIError) SafeToFallback() bool {
	if e == nil {
		return false
	}
	if e.Definitive {
		return true
	}
	return e.StatusCode >= http.StatusBadRequest && e.StatusCode < http.StatusInternalServerError
}

func (e *APIError) Error() string {
	if e == nil {
		return "mnotify api error"
	}

	code := e.Code.String()
	message := strings.TrimSpace(e.Message)
	status := strings.TrimSpace(e.Status)
	if code != "" || message != "" || status != "" {
		return fmt.Sprintf("mnotify api error: status %q code %q message %q", status, code, message)
	}
	if strings.TrimSpace(e.Body) != "" {
		return fmt.Sprintf("mnotify api returned status %d: %s", e.StatusCode, strings.TrimSpace(e.Body))
	}
	return fmt.Sprintf("mnotify api returned status %d", e.StatusCode)
}

func NewClient(cfg config.ProviderConfig) *Client {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  strings.TrimSpace(cfg.APIKey),
		HTTPClient: &http.Client{
			Timeout: defaultClientTimeout,
		},
	}
}

func (c *Client) doRequest(
	ctx context.Context,
	method string,
	path string,
	payload any,
	result any,
) error {
	if c == nil {
		return errors.New("mnotify client is nil")
	}
	if strings.TrimSpace(c.BaseURL) == "" {
		return errors.New("mnotify base URL is required")
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return errors.New("mnotify API key is required")
	}
	if c.HTTPClient == nil {
		return errors.New("mnotify HTTP client is required")
	}

	var body io.Reader = http.NoBody
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode mnotify request: %w", err)
		}
		body = bytes.NewReader(data)
	}

	requestURL, err := buildRequestURL(c.BaseURL, path, c.APIKey)
	if err != nil {
		return fmt.Errorf("create mnotify request URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return fmt.Errorf("create mnotify request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("send mnotify request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
	if err != nil {
		return fmt.Errorf("read mnotify response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &APIError{
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(responseBody)),
		}
	}

	if result == nil || resp.StatusCode == http.StatusNoContent || len(bytes.TrimSpace(responseBody)) == 0 {
		return nil
	}

	if err := json.Unmarshal(responseBody, result); err != nil {
		return fmt.Errorf("decode mnotify response: %w", err)
	}

	return nil
}

func buildRequestURL(baseURL, path, apiKey string) (string, error) {
	requestURL, err := url.Parse(strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/"))
	if err != nil {
		return "", err
	}
	if requestURL.Scheme == "" || requestURL.Host == "" {
		return "", errors.New("mnotify base URL must include scheme and host")
	}

	query := requestURL.Query()
	query.Set("key", apiKey)
	requestURL.RawQuery = query.Encode()

	return requestURL.String(), nil
}
