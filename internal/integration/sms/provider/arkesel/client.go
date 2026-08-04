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
)

const (
	defaultBaseURL       = "https://sms.arkesel.com"
	maxErrorResponseBody = 32 << 10 // 32 KiB
)

type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// APIError preserves the HTTP status and upstream response body so the routing
// layer can decide whether a request is safe to retry through another provider.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) SafeToFallback() bool {
	if e == nil {
		return false
	}

	// These failures happen before Arkesel accepts the SMS, so trying the next
	// provider cannot duplicate a previously accepted message. Authentication and
	// server-side failures are intentionally excluded because they may represent a
	// misconfigured integration or an ambiguous upstream outcome.
	switch e.StatusCode {
	case http.StatusBadRequest,
		http.StatusNotFound,
		http.StatusMethodNotAllowed,
		http.StatusConflict,
		http.StatusGone,
		http.StatusLengthRequired,
		http.StatusPreconditionFailed,
		http.StatusRequestEntityTooLarge,
		http.StatusRequestURITooLong,
		http.StatusUnsupportedMediaType,
		http.StatusRequestedRangeNotSatisfiable,
		http.StatusExpectationFailed,
		http.StatusTeapot,
		http.StatusMisdirectedRequest,
		http.StatusUnprocessableEntity,
		http.StatusLocked,
		http.StatusFailedDependency,
		http.StatusTooEarly,
		http.StatusUpgradeRequired,
		http.StatusPreconditionRequired,
		http.StatusTooManyRequests,
		http.StatusRequestHeaderFieldsTooLarge,
		http.StatusUnavailableForLegalReasons:
		return true
	default:
		return false
	}
}

func (e *APIError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("arkesel api error: status code %d", e.StatusCode)
	}

	return fmt.Sprintf("arkesel api error: status code %d: %s", e.StatusCode, e.Body)
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
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) doRequest(ctx context.Context, method, path string, payload any, result any) error {
	if c == nil {
		return fmt.Errorf("arkesel client is nil")
	}
	if strings.TrimSpace(c.BaseURL) == "" {
		return fmt.Errorf("arkesel base URL is required")
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return fmt.Errorf("arkesel API key is required")
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	var body io.Reader = http.NoBody
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode arkesel request: %w", err)
		}
		body = bytes.NewReader(data)
	}

	endpoint := strings.TrimRight(c.BaseURL, "/") + "/" + strings.TrimLeft(path, "/")
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fmt.Errorf("create arkesel request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("api-key", c.APIKey)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send arkesel request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorResponseBody))
		return &APIError{
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(responseBody)),
		}
	}

	if result == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("decode arkesel response: %w", err)
	}

	return nil
}
