package httpclient

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxRedirects = 10

// NewFixedEndpointClient returns an HTTP client that permits redirects only
// within the configured HTTPS origin. Provider credentials must never be
// forwarded to a different host or downgraded to plaintext HTTP.
func NewFixedEndpointClient(endpoint string, timeout time.Duration) *http.Client {
	origin, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || !strings.EqualFold(origin.Scheme, "https") || origin.Host == "" {
		panic(fmt.Sprintf("fixed provider endpoint must be an absolute HTTPS URL: %q", endpoint))
	}

	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("provider redirect limit exceeded")
			}
			if !strings.EqualFold(request.URL.Scheme, origin.Scheme) ||
				!strings.EqualFold(request.URL.Host, origin.Host) {
				return fmt.Errorf("provider redirect outside fixed HTTPS origin is not allowed")
			}
			return nil
		},
	}
}
