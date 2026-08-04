package mnotify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

type SendRequest struct {
	Recipient    []string `json:"recipient"`
	Sender       string   `json:"sender"`
	Message      string   `json:"message"`
	IsSchedule   bool     `json:"is_schedule"`
	ScheduleDate string   `json:"schedule_date"`
}

type ResponseCode string

func (c *ResponseCode) UnmarshalJSON(data []byte) error {
	if c == nil {
		return fmt.Errorf("mnotify response code is nil")
	}

	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*c = ""
		return nil
	}

	if data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*c = ResponseCode(strings.TrimSpace(value))
		return nil
	}

	var number json.Number
	if err := json.Unmarshal(data, &number); err != nil {
		return fmt.Errorf("mnotify response code must be a string or number: %w", err)
	}
	*c = ResponseCode(number.String())
	return nil
}

func (c ResponseCode) String() string {
	return strings.TrimSpace(string(c))
}

type SendResponse struct {
	Status  string       `json:"status"`
	Code    ResponseCode `json:"code"`
	Message string       `json:"message"`
	Summary struct {
		ID            string   `json:"_id"`
		Type          string   `json:"type"`
		TotalSent     int      `json:"total_sent"`
		Contacts      int      `json:"contacts"`
		TotalRejected int      `json:"total_rejected"`
		NumbersSent   []string `json:"numbers_sent"`
		CreditUsed    int      `json:"credit_used"`
		CreditLeft    int      `json:"credit_left"`
	} `json:"summary"`
}

// CampaignStatusResponse is returned by GET /api/campaign/{campaignID}.
// The report field is an array because a campaign may contain multiple recipients.
type CampaignStatusResponse struct {
	Status string         `json:"status"`
	Report []StatusReport `json:"report"`
}

type StatusReport struct {
	ID         int64  `json:"_id"`
	Recipient  string `json:"recipient"`
	Message    string `json:"message"`
	Sender     string `json:"sender"`
	Status     string `json:"status"`
	DateSent   string `json:"date_sent"`
	CampaignID string `json:"campaign_id"`
	Retries    int    `json:"retries"`
}
