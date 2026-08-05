package emaildelivery

import "time"

var defaultRetryDelays = []time.Duration{
	5 * time.Second,
	30 * time.Second,
	2 * time.Minute,
	10 * time.Minute,
	1 * time.Hour,
	6 * time.Hour,
}

type RetryPolicy struct {
	Delays []time.Duration
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{Delays: append([]time.Duration(nil), defaultRetryDelays...)}
}

func (p RetryPolicy) Delay(delivery uint64) time.Duration {
	delays := p.Delays
	if len(delays) == 0 {
		delays = defaultRetryDelays
	}
	index := int(delivery)
	if index < 1 {
		index = 1
	}
	index--
	if index >= len(delays) {
		index = len(delays) - 1
	}
	return delays[index]
}
