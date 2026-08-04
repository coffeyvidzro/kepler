package email

import (
	"slices"
	"strings"
)

var supportedSESRegions = map[string]struct{}{
	"us-east-1":  {},
	"eu-north-1": {},
}

// NormalizeSESRegion returns a canonical supported SES region and reports
// whether the supplied value belongs to Dugble's regional deployment set.
func NormalizeSESRegion(value string) (string, bool) {
	region := strings.ToLower(strings.TrimSpace(value))
	_, supported := supportedSESRegions[region]
	return region, supported
}

// SupportedSESRegions returns a copy so callers cannot mutate shared policy.
func SupportedSESRegions() []string {
	regions := make([]string, 0, len(supportedSESRegions))
	for region := range supportedSESRegions {
		regions = append(regions, region)
	}
	slices.Sort(regions)
	return regions
}
