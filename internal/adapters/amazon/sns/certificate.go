package sns

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

const (
	defaultCertificateTimeout = 5 * time.Second
	maxCertificateBytes       = 1 << 20
)

type CertificateLoader interface {
	Load(context.Context, string) (*x509.Certificate, error)
}

type cachedCertificate struct {
	certificate *x509.Certificate
	expiresAt   time.Time
}

type signingCertificateLocation struct {
	hostname string
	filename string
}

type HTTPCertificateLoader struct {
	client *http.Client
	now    func() time.Time
	mu     sync.RWMutex
	cache  map[string]cachedCertificate
}

func NewHTTPCertificateLoader(client *http.Client) *HTTPCertificateLoader {
	configured := http.Client{}
	if client != nil {
		configured = *client
	}
	if configured.Timeout <= 0 {
		configured.Timeout = defaultCertificateTimeout
	}
	configured.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &HTTPCertificateLoader{
		client: &configured,
		now:    time.Now,
		cache:  make(map[string]cachedCertificate),
	}
}

func (l *HTTPCertificateLoader) Load(ctx context.Context, rawURL string) (*x509.Certificate, error) {
	if l == nil || l.client == nil {
		return nil, fmt.Errorf("%w: HTTP client is not configured", ErrCertificateUnavailable)
	}
	baseURL, filename, cacheKey, err := l.resolveCertificateTarget(rawURL)
	if err != nil {
		return nil, err
	}

	now := l.currentTime()
	l.mu.RLock()
	cached, ok := l.cache[cacheKey]
	l.mu.RUnlock()
	if ok && cached.expiresAt.After(now.Add(time.Minute)) {
		return cached.certificate, nil
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("%w: create request: %v", ErrCertificateUnavailable, err)
	}
	request.URL.Path = "/" + filename
	response, err := l.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: download certificate: %v", ErrCertificateUnavailable, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: unexpected HTTP status %d", ErrCertificateUnavailable, response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxCertificateBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read certificate: %v", ErrCertificateUnavailable, err)
	}
	if len(body) > maxCertificateBytes {
		return nil, fmt.Errorf("%w: certificate exceeds %d bytes", ErrInvalidCertificate, maxCertificateBytes)
	}
	block, rest := pem.Decode(body)
	if block == nil || block.Type != "CERTIFICATE" || len(strings.TrimSpace(string(rest))) != 0 {
		return nil, fmt.Errorf("%w: expected one PEM certificate", ErrInvalidCertificate)
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: parse X.509 certificate: %v", ErrInvalidCertificate, err)
	}
	if now.Before(certificate.NotBefore) || !now.Before(certificate.NotAfter) {
		return nil, fmt.Errorf("%w: certificate is not currently valid", ErrInvalidCertificate)
	}
	if _, ok := certificate.PublicKey.(*rsa.PublicKey); !ok {
		return nil, fmt.Errorf("%w: certificate does not contain an RSA public key", ErrInvalidCertificate)
	}

	l.mu.Lock()
	l.cache[cacheKey] = cachedCertificate{certificate: certificate, expiresAt: certificate.NotAfter}
	l.mu.Unlock()
	return certificate, nil
}

func (l *HTTPCertificateLoader) resolveCertificateTarget(rawURL string) (string, string, string, error) {
	location, err := parseSigningCertificateURL(rawURL)
	if err != nil {
		return "", "", "", err
	}
	baseURL, ok := trustedSNSEndpoint(location.hostname)
	if !ok {
		return "", "", "", fmt.Errorf(
			"%w: hostname %q is not configured for this endpoint",
			ErrUntrustedCertificateURL,
			location.hostname,
		)
	}
	return baseURL, location.filename, baseURL + location.filename, nil
}

func (l *HTTPCertificateLoader) currentTime() time.Time {
	if l != nil && l.now != nil {
		return l.now()
	}
	return time.Now()
}

func validateSigningCertificateURL(rawURL string) error {
	_, err := parseSigningCertificateURL(rawURL)
	return err
}

func parseSigningCertificateURL(rawURL string) (signingCertificateLocation, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return signingCertificateLocation{}, fmt.Errorf("%w: parse URL: %v", ErrUntrustedCertificateURL, err)
	}
	if parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawPath != "" || parsed.Opaque != "" || parsed.ForceQuery {
		return signingCertificateLocation{}, fmt.Errorf("%w: URL must be a direct HTTPS AWS SNS certificate URL", ErrUntrustedCertificateURL)
	}
	hostname := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if !isTrustedSNSHostname(hostname) {
		return signingCertificateLocation{}, fmt.Errorf("%w: hostname %q is not an AWS SNS endpoint", ErrUntrustedCertificateURL, hostname)
	}
	if path.Dir(parsed.Path) != "/" {
		return signingCertificateLocation{}, fmt.Errorf("%w: certificate must be located at the endpoint root", ErrUntrustedCertificateURL)
	}
	filename := path.Base(parsed.Path)
	if !strings.HasPrefix(filename, "SimpleNotificationService-") || !strings.HasSuffix(filename, ".pem") {
		return signingCertificateLocation{}, fmt.Errorf("%w: unexpected certificate filename", ErrUntrustedCertificateURL)
	}
	identifier := strings.TrimSuffix(strings.TrimPrefix(filename, "SimpleNotificationService-"), ".pem")
	if identifier == "" || !isSafeCertificateIdentifier(identifier) {
		return signingCertificateLocation{}, fmt.Errorf("%w: invalid certificate identifier", ErrUntrustedCertificateURL)
	}
	return signingCertificateLocation{hostname: hostname, filename: filename}, nil
}

func isTrustedSNSHostname(hostname string) bool {
	_, ok := trustedSNSEndpoint(hostname)
	return ok
}

func isDNSLabel(value string) bool {
	if value == "" || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
			return false
		}
	}
	return true
}

func isSafeCertificateIdentifier(value string) bool {
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '-' && character != '_' {
			return false
		}
	}
	return true
}
