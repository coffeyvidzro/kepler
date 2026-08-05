package httpclient

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxRedirects = 10

// ForFixedEndpoint returns a copy of client that permits redirects only within
// the configured HTTPS origin. Provider credentials must never be forwarded to
// a different host or downgraded to plaintext HTTP.
func ForFixedEndpoint(endpoint string, client *http.Client, defaultTimeout time.Duration) *http.Client {
	origin, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || !strings.EqualFold(origin.Scheme, "https") || origin.Host == "" {
		panic(fmt.Sprintf("fixed provider endpoint must be an absolute HTTPS URL: %q", endpoint))
	}

	secured := &http.Client{}
	if client != nil {
		*secured = *client
	}
	if secured.Timeout <= 0 {
		secured.Timeout = defaultTimeout
	}
	secured.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("provider redirect limit exceeded")
		}
		if !strings.EqualFold(request.URL.Scheme, origin.Scheme) ||
			!strings.EqualFold(request.URL.Host, origin.Host) {
			return fmt.Errorf("provider redirect outside fixed HTTPS origin is not allowed")
		}
		return nil
	}
	return secured
}
