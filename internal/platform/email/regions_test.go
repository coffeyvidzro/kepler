package email

import (
	"slices"
	"testing"
)

func TestNormalizeSESRegion(t *testing.T) {
	region, ok := NormalizeSESRegion(" EU-NORTH-1 ")
	if !ok || region != "eu-north-1" {
		t.Fatalf("NormalizeSESRegion() = %q, %v", region, ok)
	}
	for _, unsupported := range []string{"eu-west-1", "ap-northeast-1", "sa-east-1", "af-south-1"} {
		if region, ok := NormalizeSESRegion(unsupported); ok || region != unsupported {
			t.Fatalf("unsupported region = %q, %v", region, ok)
		}
	}
}

func TestSupportedSESRegionsReturnsSortedCopy(t *testing.T) {
	regions := SupportedSESRegions()
	want := []string{"eu-north-1", "us-east-1"}
	if !slices.Equal(regions, want) {
		t.Fatalf("regions = %v, want %v", regions, want)
	}
	regions[0] = "modified"
	if slices.Contains(SupportedSESRegions(), "modified") {
		t.Fatal("caller mutated shared supported region policy")
	}
}
