package email

import (
	"context"
	"errors"
	"net"
	"strings"
)

const (
	RecordDKIM = "DKIM"
	RecordSPF  = "SPF"

	RecordTypeTXT = "TXT"
	RecordTypeMX  = "MX"

	RecordStatusPending  = "pending"
	RecordStatusVerified = "verified"
	RecordStatusFailed   = "failed"
)

type Address struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type Attachment struct {
	Content     string `json:"content,omitempty"`
	Filename    string `json:"filename,omitempty"`
	Path        string `json:"path,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	ContentID   string `json:"content_id,omitempty"`
}

type Message struct {
	MessageID string `json:"message_id,omitempty"`
	AttemptID string `json:"attempt_id,omitempty"`

	Provider         string `json:"provider,omitempty"`
	Region           string `json:"region,omitempty"`
	Stream           string `json:"stream,omitempty"`
	ConfigurationSet string `json:"configuration_set,omitempty"`
	SESTenantName    string `json:"ses_tenant_name,omitempty"`

	From        Address           `json:"from"`
	ReplyTo     []Address         `json:"reply_to,omitempty"`
	To          []Address         `json:"to"`
	CC          []Address         `json:"cc,omitempty"`
	BCC         []Address         `json:"bcc,omitempty"`
	Subject     string            `json:"subject"`
	HTML        string            `json:"html,omitempty"`
	Text        string            `json:"text,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Attachments []Attachment      `json:"attachments,omitempty"`
}

type Result struct {
	Provider  string
	MessageID string
}

type Sender interface {
	Send(context.Context, Message) (Result, error)
}

type VerificationRecord struct {
	Record   string `json:"record"`
	Name     string `json:"name"`
	Value    string `json:"value"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	TTL      string `json:"ttl"`
	Priority *int   `json:"priority,omitempty"`
}

type DomainProvisionRequest struct {
	Domain           string
	Region           string
	CustomReturnPath string
	SESTenantName    string
}

type DomainStatus struct {
	IdentityVerified bool
	DKIMVerified     bool
	MailFromVerified bool
}

type DomainProvider interface {
	ProvisionDomain(context.Context, DomainProvisionRequest) ([]VerificationRecord, error)
	GetDomainStatus(context.Context, string, string) (DomainStatus, error)
	DeleteDomain(context.Context, string, string) error
}

type SendError struct {
	Code              string
	Retryable         bool
	SubmissionUnknown bool
	Err               error
}

func (e *SendError) Error() string {
	if e == nil || e.Err == nil {
		return "email send failed"
	}
	return e.Err.Error()
}

func (e *SendError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func NewSendError(code string, retryable bool, err error) error {
	return &SendError{Code: normalizeCode(code), Retryable: retryable, Err: err}
}

func NewSubmissionUnknownError(code string, err error) error {
	return &SendError{Code: normalizeCode(code), SubmissionUnknown: true, Err: err}
}

func IsSubmissionUnknown(err error) bool {
	if err == nil {
		return false
	}
	var sendError *SendError
	if errors.As(err, &sendError) {
		return sendError.SubmissionUnknown
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var networkError net.Error
	return errors.As(err, &networkError)
}

func IsRetryable(err error) bool {
	if err == nil || IsSubmissionUnknown(err) {
		return false
	}
	var sendError *SendError
	if errors.As(err, &sendError) {
		return sendError.Retryable
	}
	return false
}

func FailureCode(err error) string {
	var sendError *SendError
	if errors.As(err, &sendError) && sendError.Code != "" {
		return sendError.Code
	}
	return "provider_rejected"
}

func normalizeCode(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	code = strings.ReplaceAll(code, "-", "_")
	code = strings.ReplaceAll(code, " ", "_")
	return code
}
