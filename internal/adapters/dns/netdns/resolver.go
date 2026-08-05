package netdns

import (
	"context"
	"net"
	"strings"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/awsses"
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
func (resolver *Resolver) Verify(
	ctx context.Context,
	domain string,
	record platformemail.VerificationRecord,
) bool {
	if ctx == nil || resolver == nil || resolver.resolver == nil {
		return false
	}

	name := fullyQualifiedName(domain, record.Name)
	if name == "" {
		return false
	}
	switch record.Type {
	case platformemail.RecordTypeTXT:
		values, err := resolver.resolver.LookupTXT(ctx, name)
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
		values, err := resolver.resolver.LookupMX(ctx, name)
		if err != nil {
			return false
		}
		expected := normalizeHostname(record.Value)
		for _, value := range values {
			if normalizeHostname(value.Host) == expected &&
				(record.Priority == nil || int(value.Pref) == *record.Priority) {
				return true
			}
		}
	}

	return false
}

func fullyQualifiedName(domain string, recordName string) string {
	domain = normalizeHostname(domain)
	recordName = normalizeHostname(recordName)
	if domain == "" {
		return ""
	}
	if recordName == "" || recordName == "@" {
		return domain
	}
	if recordName == domain || strings.HasSuffix(recordName, "."+domain) {
		return recordName
	}
	return recordName + "." + domain
}

func normalizeHostname(value string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
}

func normalizeValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "\"")
	value = strings.TrimSuffix(value, "\"")
	return strings.Join(strings.Fields(value), " ")
}
