package smsdelivery

import "github.com/google/uuid"

const (
	DeliverSubject      = "dugble.job.sms.deliver.v1"
	DeliverConsumerName = "sms-delivery-v1"
	DeliverDLQSubject   = "dugble.dlq.sms.deliver.v1"
)

type DeliverCommand struct {
	EventID   uuid.UUID `json:"event_id"`
	MessageID uuid.UUID `json:"message_id"`
	TeamID    uuid.UUID `json:"team_id"`
}
