package domain

import (
	"regexp"
	"strings"

	"github.com/google/uuid"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

const maxDomainLength = 253

var (
	domainPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)
	labelPattern  = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
)

func validateCreate(req CreateRequest) (string, string, string, error) {
	domainName := normalizeDomain(req.Domain)
	region := strings.ToLower(strings.TrimSpace(req.Region))
	returnPath := strings.ToLower(strings.TrimSpace(req.CustomReturnPath))
	if region == "" {
		region = DefaultRegion
	}
	if returnPath == "" {
		returnPath = DefaultCustomReturnPath
	}
	if domainName == "" {
		return "", "", "", apperrors.NewBadRequest("Sender domain is required")
	}
	if len(domainName) > maxDomainLength || !domainPattern.MatchString(domainName) {
		return "", "", "", apperrors.NewBadRequest("Sender domain must be a valid domain name")
	}
	if err := validateRegion(region); err != nil {
		return "", "", "", err
	}
	if !labelPattern.MatchString(returnPath) {
		return "", "", "", apperrors.NewBadRequest("Custom return path must be a valid DNS label")
	}
	return domainName, region, returnPath, nil
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
