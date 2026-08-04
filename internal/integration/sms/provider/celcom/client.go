package celcom

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/coffeyvidzro/dugble/server/internal/config"
)

const (
	defaultBaseURL       = "https://isms.celcomafrica.com"
	defaultClientTimeout = 30 * time.Second
	maxResponseBodyBytes = 1 << 20
)

type Client struct {
	BaseURL    string
	APIKey     string
	PartnerID  string
	HTTPClient *http.Client
}

type APIError struct {
	HTTPStatus  int
	Code        int
	Description string
	Body        string
}

func (e *APIError) Error() string {
	if e == nil {
		return "celcom api error"
	}
	if strings.TrimSpace(e.Description) != "" {
		return fmt.Sprintf("celcom api error: code %d: %s", e.Code, e.Description)
	}
	if strings.TrimSpace(e.Body) != "" {
		return fmt.Sprintf("celcom api error: status %d: %s", e.HTTPStatus, e.Body)
	}
	if e.Code != 0 {
		return fmt.Sprintf("celcom api error: code %d", e.Code)
	}
	return fmt.Sprintf("celcom api error: status %d", e.HTTPStatus)
}

func (e *APIError) SafeToFallback() bool {
	if e == nil {
		return false
	}
	if e.Code != 0 {
		switch e.Code {
		case 1001, 1002, 1003, 1004, 1006, 1008, 1009, 1010, 4091, 4092, 4093:
			return true
		default:
			return false
		}
	}
	return e.HTTPStatus >= 400 && e.HTTPStatus < 500 && e.HTTPStatus != http.StatusUnauthorized && e.HTTPStatus != http.StatusForbidden
}

func NewClient(cfg config.CelcomConfig) *Client {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		APIKey:     strings.TrimSpace(cfg.APIKey),
		PartnerID:  strings.TrimSpace(cfg.PartnerID),
		HTTPClient: &http.Client{Timeout: defaultClientTimeout},
	}
}

func (c *Client) doRequest(ctx context.Context, path string, payload any, result any) error {
	if c == nil {
		return errors.New("celcom client is nil")
	}
	if strings.TrimSpace(c.BaseURL) == "" {
		return errors.New("celcom base URL is required")
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return errors.New("celcom API key is required")
	}
	if strings.TrimSpace(c.PartnerID) == "" {
		return errors.New("celcom partner ID is required")
	}
	if c.HTTPClient == nil {
		return errors.New("celcom HTTP client is required")
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode celcom request: %w", err)
	}
	endpoint := strings.TrimRight(c.BaseURL, "/") + "/" + strings.TrimLeft(path, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create celcom request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("send celcom request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
	if err != nil {
		return fmt.Errorf("read celcom response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &APIError{HTTPStatus: resp.StatusCode, Body: strings.TrimSpace(string(responseBody))}
	}
	if result == nil || len(bytes.TrimSpace(responseBody)) == 0 {
		return nil
	}
	if err := json.Unmarshal(responseBody, result); err != nil {
		return fmt.Errorf("decode celcom response: %w", err)
	}
	return nil
}
