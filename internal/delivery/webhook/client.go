package webhook

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

const maxResponseBodyBytes = 4 * 1024

type HTTPClient interface {
	Post(context.Context, string, http.Header, []byte) (HTTPResponse, error)
}

type Client struct {
	httpClient *http.Client
}

func NewClient(timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, fmt.Errorf("split webhook address: %w", err)
			}
			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, fmt.Errorf("resolve webhook host: %w", err)
			}
			if len(ips) == 0 {
				return nil, errors.New("webhook host did not resolve")
			}
			for _, ip := range ips {
				if !publicIP(ip) {
					return nil, errors.New("webhook host resolves to a non-public address")
				}
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: timeout,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
	}
	return &Client{httpClient: &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

func (client *Client) Post(ctx context.Context, rawURL string, headers http.Header, body []byte) (HTTPResponse, error) {
	if client == nil || client.httpClient == nil {
		return HTTPResponse{}, ErrClientNotConfigured
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return HTTPResponse{}, errors.New("webhook URL must be an absolute HTTPS URL without user information")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(body))
	if err != nil {
		return HTTPResponse{}, fmt.Errorf("create webhook request: %w", err)
	}
	request.Header = headers.Clone()
	response, err := client.httpClient.Do(request)
	if err != nil {
		return HTTPResponse{}, fmt.Errorf("send webhook request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBodyBytes+1))
	if err != nil {
		return HTTPResponse{}, fmt.Errorf("read webhook response: %w", err)
	}
	if len(responseBody) > maxResponseBodyBytes {
		responseBody = responseBody[:maxResponseBodyBytes]
	}
	return HTTPResponse{
		StatusCode: response.StatusCode,
		Body:       string(responseBody),
		Header:     response.Header.Clone(),
	}, nil
}

func publicIP(ip net.IP) bool {
	return ip.IsGlobalUnicast() &&
		!ip.IsPrivate() &&
		!ip.IsLoopback() &&
		!ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast() &&
		!ip.IsUnspecified()
}
