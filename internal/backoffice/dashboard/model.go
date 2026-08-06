package dashboard

type Stats struct {
	Users            int64
	Teams            int64
	SMSToday         int64
	FailedSMS24Hours int64
	PendingSenderIDs int64
	PendingDomains   int64
}
