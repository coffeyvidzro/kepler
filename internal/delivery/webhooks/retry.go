package webhookdelivery

import "time"

var defaultRetrySchedule = []time.Duration{
	30 * time.Second,
	2 * time.Minute,
	10 * time.Minute,
	30 * time.Minute,
	2 * time.Hour,
	8 * time.Hour,
	24 * time.Hour,
}

type RetryPolicy struct {
	Schedule []time.Duration
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{Schedule: append([]time.Duration(nil), defaultRetrySchedule...)}
}

func (p RetryPolicy) Next(attempt int, now time.Time) (time.Time, bool) {
	schedule := p.Schedule
	if len(schedule) == 0 {
		schedule = defaultRetrySchedule
	}
	if attempt < 1 || attempt > len(schedule) {
		return time.Time{}, false
	}
	return now.UTC().Add(schedule[attempt-1]), true
}
