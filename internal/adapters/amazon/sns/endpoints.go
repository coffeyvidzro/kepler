package sns

// trustedSNSEndpoints is an explicit allowlist of Amazon SNS HTTPS endpoints.
// New AWS Regions must be added deliberately so outbound requests fail closed.
var trustedSNSEndpoints = func() map[string]string {
	commercialRegions := []string{
		"af-south-1",
		"ap-east-1",
		"ap-east-2",
		"ap-northeast-1",
		"ap-northeast-2",
		"ap-northeast-3",
		"ap-south-1",
		"ap-south-2",
		"ap-southeast-1",
		"ap-southeast-2",
		"ap-southeast-3",
		"ap-southeast-4",
		"ap-southeast-5",
		"ap-southeast-6",
		"ap-southeast-7",
		"ca-central-1",
		"ca-west-1",
		"eu-central-1",
		"eu-central-2",
		"eu-north-1",
		"eu-south-1",
		"eu-south-2",
		"eu-west-1",
		"eu-west-2",
		"eu-west-3",
		"il-central-1",
		"me-central-1",
		"me-south-1",
		"mx-central-1",
		"sa-east-1",
		"us-east-1",
		"us-east-2",
		"us-gov-east-1",
		"us-gov-west-1",
		"us-west-1",
		"us-west-2",
	}
	chinaRegions := []string{"cn-north-1", "cn-northwest-1"}

	endpoints := make(map[string]string, len(commercialRegions)+len(chinaRegions)+1)
	endpoints["sns.amazonaws.com"] = "https://sns.amazonaws.com/"
	for _, region := range commercialRegions {
		host := "sns." + region + ".amazonaws.com"
		endpoints[host] = "https://" + host + "/"
	}
	for _, region := range chinaRegions {
		host := "sns." + region + ".amazonaws.com.cn"
		endpoints[host] = "https://" + host + "/"
	}
	return endpoints
}()

func trustedSNSEndpoint(hostname string) (string, bool) {
	endpoint, ok := trustedSNSEndpoints[hostname]
	return endpoint, ok
}
