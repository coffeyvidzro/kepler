package email

import (
	"errors"
	"strings"
)

const (
	internalStreamHeader           = "X-Dugble-Internal-Email-Stream"
	internalConfigurationSetHeader = "X-Dugble-Internal-SES-Configuration-Set"
	internalSESTenantHeader        = "X-Dugble-Internal-SES-Tenant"

	SystemSESTenantName           = "dugble-system"
	CustomerSandboxSESTenantName  = "dugble-t-sandbox"
	CustomerOnboardingIdentity    = "onboarding@runnage.dev"
	CustomerOnboardingSESIdentity = "runnage.dev"
)

// DeliveryRoute is the immutable provider route selected when a message is
// accepted. It is persisted with the message and must not be accepted directly
// from public API callers.
type DeliveryRoute struct {
	Stream           string
	ConfigurationSet string
	SESTenantName    string
}

// SystemDeliveryRoute is reserved for email owned by the Dugble product, such
// as authentication, security, and team-notification messages.
func SystemDeliveryRoute() DeliveryRoute {
	return DeliveryRoute{
		Stream:           "transactional",
		ConfigurationSet: "dugble-transactional",
		SESTenantName:    SystemSESTenantName,
	}
}

// CustomerSandboxDeliveryRoute is reserved for restricted customer test email
// sent with onboarding@runnage.dev. Sandbox email is always transactional and
// uses a tenant separate from Dugble system notifications and customer domains.
func CustomerSandboxDeliveryRoute() DeliveryRoute {
	return DeliveryRoute{
		Stream:           "transactional",
		ConfigurationSet: "dugble-transactional",
		SESTenantName:    CustomerSandboxSESTenantName,
	}
}

// CustomerDeliveryRoute selects a shared product configuration set and binds
// it to one explicit customer SES tenant. Customer routes can never target the
// system or sandbox tenants or silently omit tenant isolation.
func CustomerDeliveryRoute(stream, tenantName string) (DeliveryRoute, error) {
	tenantName = strings.TrimSpace(tenantName)
	if tenantName == "" {
		return DeliveryRoute{}, errors.New("customer SES tenant is required")
	}
	if strings.EqualFold(tenantName, SystemSESTenantName) {
		return DeliveryRoute{}, errors.New("dugble-system is reserved for platform email")
	}
	if strings.EqualFold(tenantName, CustomerSandboxSESTenantName) {
		return DeliveryRoute{}, errors.New("dugble-t-sandbox is reserved for restricted test email")
	}

	route := DeliveryRoute{SESTenantName: tenantName}
	switch strings.ToLower(strings.TrimSpace(stream)) {
	case "transactional":
		route.Stream = "transactional"
		route.ConfigurationSet = "dugble-transactional"
	case "marketing":
		route.Stream = "marketing"
		route.ConfigurationSet = "dugble-marketing"
	default:
		return DeliveryRoute{}, errors.New("customer email stream must be transactional or marketing")
	}
	return route, nil
}

// BuiltInDeliveryRoute remains as a compatibility helper for Dugble-owned
// routes. Customer API email must use CustomerDeliveryRoute instead.
func BuiltInDeliveryRoute(stream string) DeliveryRoute {
	if strings.EqualFold(strings.TrimSpace(stream), "marketing") {
		return DeliveryRoute{
			Stream:           "marketing",
			ConfigurationSet: "dugble-marketing",
			SESTenantName:    SystemSESTenantName,
		}
	}
	return SystemDeliveryRoute()
}

// PersistDeliveryRoute returns a copy of headers containing server-owned route
// metadata. Existing values for these internal keys are always overwritten.
func PersistDeliveryRoute(headers map[string]string, route DeliveryRoute) map[string]string {
	result := make(map[string]string, len(headers)+3)
	for key, value := range headers {
		if isInternalRouteHeader(key) {
			continue
		}
		result[key] = value
	}
	result[internalStreamHeader] = strings.TrimSpace(route.Stream)
	result[internalConfigurationSetHeader] = strings.TrimSpace(route.ConfigurationSet)
	result[internalSESTenantHeader] = strings.TrimSpace(route.SESTenantName)
	return result
}

// ExtractDeliveryRoute removes server-owned route metadata before application
// headers are rendered into the MIME message.
func ExtractDeliveryRoute(headers map[string]string) (DeliveryRoute, map[string]string) {
	route := DeliveryRoute{}
	result := make(map[string]string, len(headers))
	for key, value := range headers {
		switch {
		case strings.EqualFold(key, internalStreamHeader):
			route.Stream = strings.TrimSpace(value)
		case strings.EqualFold(key, internalConfigurationSetHeader):
			route.ConfigurationSet = strings.TrimSpace(value)
		case strings.EqualFold(key, internalSESTenantHeader):
			route.SESTenantName = strings.TrimSpace(value)
		default:
			result[key] = value
		}
	}
	return route, result
}

func isInternalRouteHeader(key string) bool {
	return strings.EqualFold(key, internalStreamHeader) ||
		strings.EqualFold(key, internalConfigurationSetHeader) ||
		strings.EqualFold(key, internalSESTenantHeader)
}
