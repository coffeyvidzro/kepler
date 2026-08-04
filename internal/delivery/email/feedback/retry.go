package feedback

import "time"

type RetryPolicy struct {
	Delays []time.Duration
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{Delays: []time.Duration{
		5 * time.Second,
		30 * time.Second,
		2 * time.Minute,
		10 * time.Minute,
		30 * time.Minute,
		1 * time.Hour,
	}}
}

func (p RetryPolicy) Delay(delivered uint64) time.Duration {
	if len(p.Delays) == 0 {
		p = DefaultRetryPolicy()
	}
	if delivered == 0 {
		return p.Delays[0]
	}
	index := int(delivered - 1)
	if index >= len(p.Delays) {
		index = len(p.Delays) - 1
	}
	return p.Delays[index]
}
