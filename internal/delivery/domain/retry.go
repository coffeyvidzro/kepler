package domainreconciliation

import (
	"time"

	"github.com/google/uuid"
)

func nextCheckDelay(attempt int32, id uuid.UUID) time.Duration {
	var delay time.Duration
	switch attempt {
	case 0, 1:
		delay = time.Minute
	case 2:
		delay = 2 * time.Minute
	case 3:
		delay = 5 * time.Minute
	case 4:
		delay = 10 * time.Minute
	case 5:
		delay = 30 * time.Minute
	default:
		delay = time.Hour << min(attempt-6, 2)
		if delay > 6*time.Hour {
			delay = 6 * time.Hour
		}
	}
	return jitter(delay, id)
}

func jitter(delay time.Duration, id uuid.UUID) time.Duration {
	jitterPercent := int(id[0])%21 - 10
	return delay + time.Duration(jitterPercent)*delay/100
}
