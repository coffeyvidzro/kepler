package domain

import (
	"regexp"
	"strings"

	"github.com/google/uuid"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/awsses"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

const maxDomainLength = 253

var (
	domainPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)
	labelPattern  = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
)

func validateCreate(req CreateRequest) (string, string, DomainConfiguration, error) {
	name := req.Name
	if strings.TrimSpace(name) == "" {
		name = req.Domain
	}
	return validateDomainConfiguration(
		name,
		req.Region,
		req.CustomReturnPath,
		req.OpenTracking,
		req.ClickTracking,
		req.TrackingSubdomain,
		req.TLS,
		req.Capabilities,
	)
}

func validateClaim(req ClaimRequest) (string, string, DomainConfiguration, error) {
	return validateDomainConfiguration(
		req.Name,
		req.Region,
		req.CustomReturnPath,
		req.OpenTracking,
		req.ClickTracking,
		req.TrackingSubdomain,
		req.TLS,
		req.Capabilities,
	)
}

func validateDomainConfiguration(
	name, region, customReturnPath string,
	openTracking, clickTracking *bool,
	trackingSubdomain *string,
	tls string,
	capabilities *Capabilities,
) (string, string, DomainConfiguration, error) {
	domainName := normalizeDomain(name)
	region = strings.ToLower(strings.TrimSpace(region))
	returnPath := strings.ToLower(strings.TrimSpace(customReturnPath))
	if region == "" {
		return "", "", DomainConfiguration{}, apperrors.NewBadRequest("Sender domain region is required")
	}
	if returnPath == "" {
		returnPath = DefaultCustomReturnPath
	}
	if domainName == "" {
		return "", "", DomainConfiguration{}, apperrors.NewBadRequest("Sender domain is required")
	}
	if len(domainName) > maxDomainLength || !domainPattern.MatchString(domainName) {
		return "", "", DomainConfiguration{}, apperrors.NewBadRequest("Sender domain must be a valid domain name")
	}
	if err := validateRegion(region); err != nil {
		return "", "", DomainConfiguration{}, err
	}
	if !labelPattern.MatchString(returnPath) {
		return "", "", DomainConfiguration{}, apperrors.NewBadRequest("Custom return path must be a valid DNS label")
	}

	configuration := DomainConfiguration{
		OpenTracking:     true,
		ClickTracking:    false,
		TLS:              DefaultTLSMode,
		Capabilities:     Capabilities{Sending: true, Receiving: false},
		CustomReturnPath: returnPath,
	}
	if openTracking != nil {
		configuration.OpenTracking = *openTracking
	}
	if clickTracking != nil {
		configuration.ClickTracking = *clickTracking
	}
	if trackingSubdomain != nil {
		value := strings.ToLower(strings.TrimSpace(*trackingSubdomain))
		if value == "" || !labelPattern.MatchString(value) {
			return "", "", DomainConfiguration{}, apperrors.NewBadRequest("Tracking subdomain must be a valid DNS label")
		}
		configuration.TrackingSubdomain = &value
	}
	if strings.TrimSpace(tls) != "" {
		configuration.TLS = strings.ToLower(strings.TrimSpace(tls))
	}
	if configuration.TLS != "opportunistic" && configuration.TLS != "enforced" {
		return "", "", DomainConfiguration{}, apperrors.NewBadRequest("TLS must be opportunistic or enforced")
	}
	if capabilities != nil {
		configuration.Capabilities = *capabilities
	}
	if !configuration.Capabilities.Sending && !configuration.Capabilities.Receiving {
		return "", "", DomainConfiguration{}, apperrors.NewBadRequest("At least one domain capability must be enabled")
	}
	if configuration.Capabilities.Receiving {
		return "", "", DomainConfiguration{}, apperrors.NewBadRequest("Receiving capability is not supported")
	}
	return domainName, region, configuration, nil
}

func validateUpdate(current SenderDomain, req UpdateRequest) (DomainConfiguration, error) {
	configuration := DomainConfiguration{
		OpenTracking:      current.OpenTracking,
		ClickTracking:     current.ClickTracking,
		TrackingSubdomain: current.TrackingSubdomain,
		TLS:               current.TLS,
		Capabilities:      current.Capabilities,
		CustomReturnPath:  current.CustomReturnPath,
	}
	if req.OpenTracking != nil {
		configuration.OpenTracking = *req.OpenTracking
	}
	if req.ClickTracking != nil {
		configuration.ClickTracking = *req.ClickTracking
	}
	if req.TrackingSubdomain != nil {
		value := strings.ToLower(strings.TrimSpace(*req.TrackingSubdomain))
		if value == "" {
			if current.TrackingSubdomain != nil {
				return DomainConfiguration{}, apperrors.NewBadRequest("Tracking subdomain cannot be removed after it is configured")
			}
			configuration.TrackingSubdomain = nil
		} else {
			if !labelPattern.MatchString(value) {
				return DomainConfiguration{}, apperrors.NewBadRequest("Tracking subdomain must be a valid DNS label")
			}
			configuration.TrackingSubdomain = &value
		}
	}
	if req.TLS != nil {
		configuration.TLS = strings.ToLower(strings.TrimSpace(*req.TLS))
		if configuration.TLS != "opportunistic" && configuration.TLS != "enforced" {
			return DomainConfiguration{}, apperrors.NewBadRequest("TLS must be opportunistic or enforced")
		}
	}
	if req.Capabilities != nil {
		configuration.Capabilities = *req.Capabilities
	}
	if !configuration.Capabilities.Sending && !configuration.Capabilities.Receiving {
		return DomainConfiguration{}, apperrors.NewBadRequest("At least one domain capability must be enabled")
	}
	if configuration.Capabilities.Receiving {
		return DomainConfiguration{}, apperrors.NewBadRequest("Receiving capability is not supported")
	}
	return configuration, nil
}

func validateRegion(region string) error {
	if _, ok := platformemail.NormalizeSESRegion(region); !ok {
		return apperrors.NewBadRequest("Sender domain region is not supported")
	}
	return nil
}

func normalizeDomain(value string) string {
	domainName := strings.TrimSpace(strings.ToLower(value))
	domainName = strings.TrimPrefix(domainName, "http://")
	domainName = strings.TrimPrefix(domainName, "https://")
	domainName = strings.TrimSuffix(domainName, ".")
	if before, _, ok := strings.Cut(domainName, "/"); ok {
		domainName = before
	}
	return domainName
}

func parseDomainID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return uuid.Nil, apperrors.NewBadRequest("Sender domain id must be a valid UUID")
	}
	return id, nil
}
