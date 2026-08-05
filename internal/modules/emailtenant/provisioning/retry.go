package tenantprovision

import "time"

var defaultRetryBackOff = []time.Duration{
	time.Second,
	5 * time.Second,
	30 * time.Second,
	2 * time.Minute,
	5 * time.Minute,
	15 * time.Minute,
}

func DefaultRetryBackOff() []time.Duration {
	return append([]time.Duration(nil), defaultRetryBackOff...)
}

func normalizeRetryBackOff(delays []time.Duration, maxDeliver int) []time.Duration {
	if maxDeliver <= 0 || len(delays) == 0 {
		return nil
	}
	result := make([]time.Duration, maxDeliver)
	for index := range result {
		source := index
		if source >= len(delays) {
			source = len(delays) - 1
		}
		result[index] = delays[source]
	}
	return result
}

func retryDelay(delays []time.Duration, delivered uint64) time.Duration {
	index := int(delivered) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(delays) {
		index = len(delays) - 1
	}
	return delays[index]
}
