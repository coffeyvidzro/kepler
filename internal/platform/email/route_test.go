package email

import "testing"

func TestSystemDeliveryRouteUsesSystemTenant(t *testing.T) {
	route := SystemDeliveryRoute()
	if route.Stream != "transactional" || route.ConfigurationSet != "dugble-transactional" || route.SESTenantName != SystemSESTenantName {
		t.Fatalf("unexpected system route: %#v", route)
	}
}

func TestCustomerDeliveryRouteUsesCustomerTenant(t *testing.T) {
	route, err := CustomerDeliveryRoute("marketing", " dugble-t-customer ")
	if err != nil {
		t.Fatalf("CustomerDeliveryRoute(): %v", err)
	}
	if route.Stream != "marketing" || route.ConfigurationSet != "dugble-marketing" || route.SESTenantName != "dugble-t-customer" {
		t.Fatalf("unexpected customer route: %#v", route)
	}
}

func TestCustomerDeliveryRouteRejectsSystemTenant(t *testing.T) {
	if _, err := CustomerDeliveryRoute("transactional", SystemSESTenantName); err == nil {
		t.Fatal("expected customer route to reject dugble-system")
	}
}

func TestCustomerDeliveryRouteRejectsMissingTenant(t *testing.T) {
	if _, err := CustomerDeliveryRoute("transactional", " "); err == nil {
		t.Fatal("expected customer route to require a tenant")
	}
}

func TestPersistAndExtractDeliveryRoute(t *testing.T) {
	route, err := CustomerDeliveryRoute("transactional", "dugble-t-customer")
	if err != nil {
		t.Fatalf("CustomerDeliveryRoute(): %v", err)
	}
	headers := PersistDeliveryRoute(map[string]string{
		"X-Customer":                              "value",
		"x-dugble-internal-email-stream":          "spoofed",
		"X-Dugble-Internal-SES-Tenant":            SystemSESTenantName,
		"X-Dugble-Internal-SES-Configuration-Set": "spoofed",
	}, route)

	persisted, applicationHeaders := ExtractDeliveryRoute(headers)
	if persisted.Stream != "transactional" || persisted.ConfigurationSet != "dugble-transactional" || persisted.SESTenantName != "dugble-t-customer" {
		t.Fatalf("unexpected delivery route: %#v", persisted)
	}
	if persisted.SESTenantName == SystemSESTenantName {
		t.Fatal("customer route fell back to dugble-system")
	}
	if len(applicationHeaders) != 1 || applicationHeaders["X-Customer"] != "value" {
		t.Fatalf("unexpected application headers: %#v", applicationHeaders)
	}
}
