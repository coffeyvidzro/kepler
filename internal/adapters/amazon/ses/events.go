package ses

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrInvalidEvent = errors.New("invalid SES feedback event")

type SubscriptionTopicPreference struct {
	TopicName          string `json:"topic_name"`
	SubscriptionStatus string `json:"subscription_status"`
}

type EventDiagnostics struct {
	BounceType            string                        `json:"bounce_type,omitempty"`
	BounceSubType         string                        `json:"bounce_sub_type,omitempty"`
	ComplaintFeedbackType string                        `json:"complaint_feedback_type,omitempty"`
	ComplaintFeedbackID   string                        `json:"complaint_feedback_id,omitempty"`
	ComplaintUserAgent    string                        `json:"complaint_user_agent,omitempty"`
	ArrivalDate           string                        `json:"arrival_date,omitempty"`
	ReportingMTA          string                        `json:"reporting_mta,omitempty"`
	SMTPResponse          string                        `json:"smtp_response,omitempty"`
	ProcessingTimeMillis  int64                         `json:"processing_time_millis,omitempty"`
	DelayType             string                        `json:"delay_type,omitempty"`
	ExpirationTime        string                        `json:"expiration_time,omitempty"`
	RejectReason          string                        `json:"reject_reason,omitempty"`
	FailureReason         string                        `json:"failure_reason,omitempty"`
	RemoteMTAIPAddress    string                        `json:"remote_mta_ip_address,omitempty"`
	IPAddress             string                        `json:"ip_address,omitempty"`
	UserAgent             string                        `json:"user_agent,omitempty"`
	Link                  string                        `json:"link,omitempty"`
	LinkTags              map[string][]string           `json:"link_tags,omitempty"`
	ContactList           string                        `json:"contact_list,omitempty"`
	SubscriptionSource    string                        `json:"subscription_source,omitempty"`
	NewTopicPreferences   []SubscriptionTopicPreference `json:"new_topic_preferences,omitempty"`
	OldTopicPreferences   []SubscriptionTopicPreference `json:"old_topic_preferences,omitempty"`
}

type RecipientDiagnostics struct {
	Email          string `json:"email"`
	Action         string `json:"action,omitempty"`
	StatusCode     string `json:"status_code,omitempty"`
	DiagnosticCode string `json:"diagnostic_code,omitempty"`
}

type FeedbackEvent struct {
	EventType            string                 `json:"event_type"`
	ProviderMessageID    string                 `json:"provider_message_id"`
	OccurredAt           time.Time              `json:"occurred_at"`
	Recipients           []string               `json:"recipients"`
	Tags                 map[string][]string    `json:"tags,omitempty"`
	InternalMessageID    string                 `json:"internal_message_id,omitempty"`
	InternalAttemptID    string                 `json:"internal_attempt_id,omitempty"`
	Diagnostics          EventDiagnostics       `json:"diagnostics,omitempty"`
	RecipientDiagnostics []RecipientDiagnostics `json:"recipient_diagnostics,omitempty"`
	BounceType           string                 `json:"bounce_type,omitempty"`
	BounceSubType        string                 `json:"bounce_sub_type,omitempty"`
	ComplaintType        string                 `json:"complaint_type,omitempty"`
	RejectReason         string                 `json:"reject_reason,omitempty"`
	FailureReason        string                 `json:"failure_reason,omitempty"`
	Payload              json.RawMessage        `json:"-"`
}

type topicPreference struct {
	TopicName          string `json:"topicName"`
	SubscriptionStatus string `json:"subscriptionStatus"`
}

type feedbackEnvelope struct {
	EventType string `json:"eventType"`
	Mail      struct {
		Timestamp   time.Time           `json:"timestamp"`
		MessageID   string              `json:"messageId"`
		Destination []string            `json:"destination"`
		Tags        map[string][]string `json:"tags"`
	} `json:"mail"`
	Send struct {
		Timestamp time.Time `json:"timestamp"`
	} `json:"send"`
	Delivery struct {
		Timestamp            time.Time `json:"timestamp"`
		Recipients           []string  `json:"recipients"`
		ProcessingTimeMillis int64     `json:"processingTimeMillis"`
		ReportingMTA         string    `json:"reportingMTA"`
		SMTPResponse         string    `json:"smtpResponse"`
		RemoteMTAIPAddress   string    `json:"remoteMtaIp"`
	} `json:"delivery"`
	DeliveryDelay struct {
		Timestamp         time.Time `json:"timestamp"`
		DelayType         string    `json:"delayType"`
		ExpirationTime    string    `json:"expirationTime"`
		DelayedRecipients []struct {
			EmailAddress   string `json:"emailAddress"`
			Status         string `json:"status"`
			DiagnosticCode string `json:"diagnosticCode"`
		} `json:"delayedRecipients"`
	} `json:"deliveryDelay"`
	Bounce struct {
		Timestamp          time.Time `json:"timestamp"`
		BounceType         string    `json:"bounceType"`
		BounceSubType      string    `json:"bounceSubType"`
		FeedbackID         string    `json:"feedbackId"`
		RemoteMTAIPAddress string    `json:"remoteMtaIp"`
		ReportingMTA       string    `json:"reportingMTA"`
		BouncedRecipients  []struct {
			EmailAddress   string `json:"emailAddress"`
			Action         string `json:"action"`
			Status         string `json:"status"`
			DiagnosticCode string `json:"diagnosticCode"`
		} `json:"bouncedRecipients"`
	} `json:"bounce"`
	Complaint struct {
		Timestamp             time.Time `json:"timestamp"`
		ComplaintFeedbackType string    `json:"complaintFeedbackType"`
		FeedbackID            string    `json:"feedbackId"`
		UserAgent             string    `json:"userAgent"`
		ArrivalDate           string    `json:"arrivalDate"`
		ComplainedRecipients  []struct {
			EmailAddress string `json:"emailAddress"`
		} `json:"complainedRecipients"`
	} `json:"complaint"`
	Reject struct {
		Timestamp time.Time `json:"timestamp"`
		Reason    string    `json:"reason"`
	} `json:"reject"`
	Failure struct {
		Timestamp    time.Time `json:"timestamp"`
		ErrorMessage string    `json:"errorMessage"`
	} `json:"failure"`
	Open struct {
		Timestamp time.Time `json:"timestamp"`
		IPAddress string    `json:"ipAddress"`
		UserAgent string    `json:"userAgent"`
	} `json:"open"`
	Click struct {
		Timestamp time.Time           `json:"timestamp"`
		IPAddress string              `json:"ipAddress"`
		UserAgent string              `json:"userAgent"`
		Link      string              `json:"link"`
		LinkTags  map[string][]string `json:"linkTags"`
	} `json:"click"`
	Subscription struct {
		ContactList         string            `json:"contactList"`
		Timestamp           time.Time         `json:"timestamp"`
		Source              string            `json:"source"`
		NewTopicPreferences []topicPreference `json:"newTopicPreferences"`
		OldTopicPreferences []topicPreference `json:"oldTopicPreferences"`
	} `json:"subscription"`
}

func ParseFeedbackEvent(message string) (FeedbackEvent, error) {
	raw := json.RawMessage(strings.TrimSpace(message))
	if len(raw) == 0 || !json.Valid(raw) {
		return FeedbackEvent{}, fmt.Errorf("%w: payload must be valid JSON", ErrInvalidEvent)
	}
	var envelope feedbackEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return FeedbackEvent{}, fmt.Errorf("%w: decode payload: %v", ErrInvalidEvent, err)
	}
	eventType, occurredAt, recipients, err := normalizeFeedbackEvent(envelope)
	if err != nil {
		return FeedbackEvent{}, err
	}
	messageID := strings.TrimSpace(envelope.Mail.MessageID)
	if messageID == "" {
		return FeedbackEvent{}, fmt.Errorf("%w: mail.messageId is required", ErrInvalidEvent)
	}
	if occurredAt.IsZero() {
		occurredAt = envelope.Mail.Timestamp
	}
	if occurredAt.IsZero() {
		return FeedbackEvent{}, fmt.Errorf("%w: event timestamp is required", ErrInvalidEvent)
	}
	if len(recipients) == 0 {
		recipients = normalizeRecipients(envelope.Mail.Destination)
	}
	tags := normalizeTags(envelope.Mail.Tags)
	diagnostics := normalizeEventDiagnostics(envelope)
	return FeedbackEvent{
		EventType: eventType, ProviderMessageID: messageID, OccurredAt: occurredAt.UTC(), Recipients: recipients,
		Tags: tags, InternalMessageID: firstTagValue(tags, "dugble_message_id"), InternalAttemptID: firstTagValue(tags, "dugble_attempt_id"),
		Diagnostics: diagnostics, RecipientDiagnostics: normalizeRecipientDiagnostics(envelope), BounceType: diagnostics.BounceType,
		BounceSubType: diagnostics.BounceSubType, ComplaintType: diagnostics.ComplaintFeedbackType, RejectReason: diagnostics.RejectReason,
		FailureReason: diagnostics.FailureReason, Payload: append(json.RawMessage(nil), raw...),
	}, nil
}

func normalizeFeedbackEvent(envelope feedbackEnvelope) (string, time.Time, []string, error) {
	switch strings.ToLower(strings.TrimSpace(envelope.EventType)) {
	case "send":
		return "send", envelope.Send.Timestamp, nil, nil
	case "delivery":
		return "delivery", envelope.Delivery.Timestamp, normalizeRecipients(envelope.Delivery.Recipients), nil
	case "deliverydelay", "delivery_delay":
		return "delivery_delay", envelope.DeliveryDelay.Timestamp, delayedRecipients(envelope.DeliveryDelay.DelayedRecipients), nil
	case "bounce":
		return "bounce", envelope.Bounce.Timestamp, bouncedRecipients(envelope.Bounce.BouncedRecipients), nil
	case "complaint":
		return "complaint", envelope.Complaint.Timestamp, complainedRecipients(envelope.Complaint.ComplainedRecipients), nil
	case "reject":
		return "reject", envelope.Reject.Timestamp, nil, nil
	case "rendering failure", "renderingfailure", "rendering_failure":
		return "rendering_failure", envelope.Failure.Timestamp, nil, nil
	case "open":
		return "open", envelope.Open.Timestamp, nil, nil
	case "click":
		return "click", envelope.Click.Timestamp, nil, nil
	case "subscription":
		return "subscription", envelope.Subscription.Timestamp, nil, nil
	default:
		return "", time.Time{}, nil, fmt.Errorf("%w: unsupported event type %q", ErrInvalidEvent, envelope.EventType)
	}
}

func normalizeEventDiagnostics(envelope feedbackEnvelope) EventDiagnostics {
	return EventDiagnostics{
		BounceType: strings.TrimSpace(envelope.Bounce.BounceType), BounceSubType: strings.TrimSpace(envelope.Bounce.BounceSubType),
		ComplaintFeedbackType: strings.TrimSpace(envelope.Complaint.ComplaintFeedbackType), ComplaintFeedbackID: strings.TrimSpace(envelope.Complaint.FeedbackID),
		ComplaintUserAgent: strings.TrimSpace(envelope.Complaint.UserAgent), ArrivalDate: strings.TrimSpace(envelope.Complaint.ArrivalDate),
		ReportingMTA: firstNonEmpty(envelope.Delivery.ReportingMTA, envelope.Bounce.ReportingMTA), SMTPResponse: strings.TrimSpace(envelope.Delivery.SMTPResponse),
		ProcessingTimeMillis: envelope.Delivery.ProcessingTimeMillis, DelayType: strings.TrimSpace(envelope.DeliveryDelay.DelayType),
		ExpirationTime: strings.TrimSpace(envelope.DeliveryDelay.ExpirationTime), RejectReason: strings.TrimSpace(envelope.Reject.Reason),
		FailureReason: strings.TrimSpace(envelope.Failure.ErrorMessage), RemoteMTAIPAddress: firstNonEmpty(envelope.Delivery.RemoteMTAIPAddress, envelope.Bounce.RemoteMTAIPAddress),
		IPAddress: firstNonEmpty(envelope.Open.IPAddress, envelope.Click.IPAddress), UserAgent: firstNonEmpty(envelope.Open.UserAgent, envelope.Click.UserAgent),
		Link: strings.TrimSpace(envelope.Click.Link), LinkTags: normalizeTags(envelope.Click.LinkTags), ContactList: strings.TrimSpace(envelope.Subscription.ContactList),
		SubscriptionSource: strings.TrimSpace(envelope.Subscription.Source), NewTopicPreferences: normalizeTopicPreferences(envelope.Subscription.NewTopicPreferences),
		OldTopicPreferences: normalizeTopicPreferences(envelope.Subscription.OldTopicPreferences),
	}
}

func normalizeTopicPreferences(values []topicPreference) []SubscriptionTopicPreference {
	result := make([]SubscriptionTopicPreference, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		name, status := strings.TrimSpace(value.TopicName), strings.TrimSpace(value.SubscriptionStatus)
		if name == "" || status == "" {
			continue
		}
		key := strings.ToLower(name) + "\x00" + strings.ToLower(status)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, SubscriptionTopicPreference{TopicName: name, SubscriptionStatus: status})
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func normalizeRecipientDiagnostics(envelope feedbackEnvelope) []RecipientDiagnostics {
	result := make([]RecipientDiagnostics, 0, len(envelope.Bounce.BouncedRecipients)+len(envelope.DeliveryDelay.DelayedRecipients))
	seen := map[string]struct{}{}
	for _, recipient := range envelope.Bounce.BouncedRecipients {
		email := normalizeRecipient(recipient.EmailAddress)
		if email == "" {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		result = append(result, RecipientDiagnostics{Email: email, Action: strings.TrimSpace(recipient.Action), StatusCode: strings.TrimSpace(recipient.Status), DiagnosticCode: strings.TrimSpace(recipient.DiagnosticCode)})
	}
	for _, recipient := range envelope.DeliveryDelay.DelayedRecipients {
		email := normalizeRecipient(recipient.EmailAddress)
		if email == "" {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		result = append(result, RecipientDiagnostics{Email: email, StatusCode: strings.TrimSpace(recipient.Status), DiagnosticCode: strings.TrimSpace(recipient.DiagnosticCode)})
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func normalizeRecipients(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = normalizeRecipient(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
func normalizeRecipient(value string) string { return strings.ToLower(strings.TrimSpace(value)) }
func normalizeTags(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string][]string, len(values))
	for key, entries := range values {
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" {
			continue
		}
		seen := map[string]struct{}{}
		for _, entry := range entries {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			if _, ok := seen[entry]; ok {
				continue
			}
			seen[entry] = struct{}{}
			result[key] = append(result[key], entry)
		}
		if len(result[key]) == 0 {
			delete(result, key)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
func firstTagValue(tags map[string][]string, key string) string {
	values := tags[strings.ToLower(strings.TrimSpace(key))]
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
func delayedRecipients(values []struct {
	EmailAddress   string `json:"emailAddress"`
	Status         string `json:"status"`
	DiagnosticCode string `json:"diagnosticCode"`
}) []string {
	recipients := make([]string, 0, len(values))
	for _, value := range values {
		recipients = append(recipients, value.EmailAddress)
	}
	return normalizeRecipients(recipients)
}
func bouncedRecipients(values []struct {
	EmailAddress   string `json:"emailAddress"`
	Action         string `json:"action"`
	Status         string `json:"status"`
	DiagnosticCode string `json:"diagnosticCode"`
}) []string {
	recipients := make([]string, 0, len(values))
	for _, value := range values {
		recipients = append(recipients, value.EmailAddress)
	}
	return normalizeRecipients(recipients)
}
func complainedRecipients(values []struct {
	EmailAddress string `json:"emailAddress"`
}) []string {
	recipients := make([]string, 0, len(values))
	for _, value := range values {
		recipients = append(recipients, value.EmailAddress)
	}
	return normalizeRecipients(recipients)
}
