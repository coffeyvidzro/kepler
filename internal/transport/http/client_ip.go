package httptransport

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/labstack/echo/v5"
)

const trustedProxyCIDRsEnv = "TRUSTED_PROXY_CIDRS"

// newClientIPExtractor returns a direct extractor unless one or more trusted
// proxy CIDRs are configured. A nil slice loads the comma-separated deployment
// setting from TRUSTED_PROXY_CIDRS; an explicit empty slice disables proxy trust.
func newClientIPExtractor(configuredCIDRs []string) (echo.IPExtractor, error) {
	cidrs := configuredCIDRs
	if cidrs == nil {
		cidrs = strings.Split(os.Getenv(trustedProxyCIDRsEnv), ",")
	}

	options := []echo.TrustOption{
		echo.TrustLoopback(false),
		echo.TrustLinkLocal(false),
		echo.TrustPrivateNet(false),
	}
	trusted := false
	for _, value := range cidrs {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		_, network, err := net.ParseCIDR(value)
		if err != nil {
			return nil, fmt.Errorf("parse trusted proxy CIDR %q: %w", value, err)
		}
		options = append(options, echo.TrustIPRange(network))
		trusted = true
	}

	if !trusted {
		return echo.ExtractIPDirect(), nil
	}
	return echo.ExtractIPFromXFFHeader(options...), nil
}
