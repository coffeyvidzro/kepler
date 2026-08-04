package email

import (
	"context"
	"net"
	"strings"
)

// DNSVerifier checks whether a provider-neutral verification record is present.
type DNSVerifier interface {
	Verify(context.Context, string, VerificationRecord) bool
}

type NetDNSVerifier struct {
	resolver *net.Resolver
}

func NewNetDNSVerifier() *NetDNSVerifier {
	return &NetDNSVerifier{resolver: net.DefaultResolver}
}

func (v *NetDNSVerifier) Verify(ctx context.Context, domain string, record VerificationRecord) bool {
	if v == nil || v.resolver == nil {
		return false
	}

	fqdn := strings.TrimSuffix(record.Name, ".") + "." + strings.TrimSuffix(domain, ".")
	switch record.Type {
	case RecordTypeTXT:
		values, err := v.resolver.LookupTXT(ctx, fqdn)
		if err != nil {
			return false
		}
		expected := normalizeDNSValue(record.Value)
		for _, value := range values {
			if normalizeDNSValue(value) == expected {
				return true
			}
		}
	case RecordTypeMX:
		values, err := v.resolver.LookupMX(ctx, fqdn)
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
	}

	return false
}

func normalizeDNSValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "\"")
	value = strings.TrimSuffix(value, "\"")
	return strings.Join(strings.Fields(value), " ")
}
