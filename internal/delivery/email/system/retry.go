package systememail

import "time"

func retryDelay(delivery uint64) time.Duration {
	if delivery < 1 {
		delivery = 1
	}
	delay := time.Duration(delivery) * time.Second
	if delay > time.Minute {
		return time.Minute
	}
	return delay
}
