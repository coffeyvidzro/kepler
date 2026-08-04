package domain

import "testing"

func TestValidateCreateNormalizesDomain(t *testing.T) {
	domainName, region, returnPath, err := validateCreate(CreateRequest{
		Domain:           " HTTPS://Mail.Example.COM/path ",
		Region:           " eu-north-1 ",
		CustomReturnPath: " Bounce ",
	})
	if err != nil {
		t.Fatalf("validateCreate returned error: %v", err)
	}
	if domainName != "mail.example.com" {
		t.Fatalf("domainName = %q, want mail.example.com", domainName)
	}
	if region != "eu-north-1" {
		t.Fatalf("region = %q, want eu-north-1", region)
	}
	if returnPath != "bounce" {
		t.Fatalf("returnPath = %q, want bounce", returnPath)
	}
}

func TestValidateCreateUsesDefaults(t *testing.T) {
	domainName, region, returnPath, err := validateCreate(CreateRequest{Domain: "example.com"})
	if err != nil {
		t.Fatalf("validateCreate returned error: %v", err)
	}
	if domainName != "example.com" || region != "eu-north-1" || returnPath != DefaultCustomReturnPath {
		t.Fatalf("validateCreate = %q, %q, %q", domainName, region, returnPath)
	}
}

func TestValidateCreateAcceptsSupportedRegions(t *testing.T) {
	regions := []string{
		"us-east-1",
		"eu-north-1",
	}

	for _, region := range regions {
		t.Run(region, func(t *testing.T) {
			_, gotRegion, _, err := validateCreate(CreateRequest{
				Domain: "example.com",
				Region: region,
			})
			if err != nil {
				t.Fatalf("validateCreate returned error: %v", err)
			}
			if gotRegion != region {
				t.Fatalf("region = %q, want %q", gotRegion, region)
			}
		})
	}
}

func TestValidateCreateRejectsInvalidDomain(t *testing.T) {
	_, _, _, err := validateCreate(CreateRequest{Domain: "not a domain"})
	if err == nil {
		t.Fatal("validateCreate returned nil error for invalid domain")
	}
}

func TestValidateCreateRejectsUnsupportedRegion(t *testing.T) {
	for _, region := range []string{"eu-west-1", "ap-northeast-1", "sa-east-1", "af-south-1"} {
		t.Run(region, func(t *testing.T) {
			_, _, _, err := validateCreate(CreateRequest{Domain: "example.com", Region: region})
			if err == nil {
				t.Fatal("validateCreate returned nil error for unsupported region")
			}
		})
	}
}

func TestValidateCreateRejectsInvalidReturnPath(t *testing.T) {
	_, _, _, err := validateCreate(CreateRequest{Domain: "example.com", CustomReturnPath: "bad.path"})
	if err == nil {
		t.Fatal("validateCreate returned nil error for invalid return path")
	}
}
