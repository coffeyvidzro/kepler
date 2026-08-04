package netdns

import (
	"context"
	"net"
	"strings"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"
)

// Resolver verifies provider-neutral DNS records using the standard library resolver.
type Resolver struct {
	resolver *net.Resolver
}

// New returns a resolver backed by net.DefaultResolver.
func New() *Resolver {
	return NewWithResolver(net.DefaultResolver)
}

// NewWithResolver returns a resolver backed by the supplied DNS resolver.
func NewWithResolver(resolver *net.Resolver) *Resolver {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	return &Resolver{resolver: resolver}
}

// Verify reports whether the expected DNS record is present for domain.
func (r *Resolver) Verify(ctx context.Context, domain string, record platformemail.VerificationRecord) bool {
	if r == nil || r.resolver == nil {
		return false
	}

	fqdn := strings.TrimSuffix(record.Name, ".") + "." + strings.TrimSuffix(domain, ".")
	switch record.Type {
	case platformemail.RecordTypeTXT:
		values, err := r.resolver.LookupTXT(ctx, fqdn)
		if err != nil {
			return false
		}
		expected := normalizeValue(record.Value)
		for _, value := range values {
			if normalizeValue(value) == expected {
				return true
			}
		}
	case platformemail.RecordTypeMX:
		values, err := r.resolver.LookupMX(ctx, fqdn)
		if err != nil {
			return false
		}
		expected := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(record.Value)), ".")
		for _, value := range values {
			if strings.TrimSuffix(strings.ToLower(value.Host), ".") == expected &&
				(record.Priority == nil || int(value.Pref) == *record.Priority) {
				return true
			}
		}

	return false
}

func normalizeValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "\"")
	value = strings.TrimSuffix(value, "\"")
	return strings.Join(strings.Fields(value), " ")
}
