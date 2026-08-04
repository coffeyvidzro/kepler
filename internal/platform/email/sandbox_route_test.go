package email

import "testing"

func TestCustomerSandboxDeliveryRoute(t *testing.T) {
	route := CustomerSandboxDeliveryRoute()
	if route.Stream != "transactional" {
		t.Fatalf("stream = %q, want transactional", route.Stream)
	}
	if route.ConfigurationSet != "dugble-transactional" {
		t.Fatalf("configuration set = %q, want dugble-transactional", route.ConfigurationSet)
	}
	if route.SESTenantName != CustomerSandboxSESTenantName {
		t.Fatalf("tenant = %q, want %q", route.SESTenantName, CustomerSandboxSESTenantName)
	}
}

func TestCustomerDeliveryRouteRejectsReservedSandboxTenant(t *testing.T) {
	if _, err := CustomerDeliveryRoute("transactional", CustomerSandboxSESTenantName); err == nil {
		t.Fatal("expected customer route to reject reserved sandbox tenant")
	}
}
