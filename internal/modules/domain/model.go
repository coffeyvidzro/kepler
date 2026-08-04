package domain

import (
	"time"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"
)

const (
	DefaultProvider         = "aws_ses"
	DefaultRegion           = "eu-north-1"
	DefaultCustomReturnPath = "send"

	StatusPending  = "pending"
	StatusVerified = "verified"
	StatusFailed   = "failed"
	StatusDisabled = "disabled"

	HealthStatusUnknown  = "unknown"
	HealthStatusHealthy  = "healthy"
	HealthStatusDegraded = "degraded"

	DefaultHealthFailureThreshold int32 = 3

	emailInfrastructureRetryAfterSeconds   = 10
	emailInfrastructureProvisioningMessage = "Customer email infrastructure is being prepared"
)

type VerificationRecord = platformemail.VerificationRecord

type SenderDomain struct {
	ID                        string               `json:"id"`
	TeamID                    string               `json:"team_id"`
	Domain                    string               `json:"name"`
	Provider                  string               `json:"provider,omitempty"`
	ProviderRegion            string               `json:"region"`
	Status                    string               `json:"status"`
	VerificationRecords       []VerificationRecord `json:"records"`
	FailureReason             *string              `json:"failure_reason,omitempty"`
	HealthStatus              string               `json:"health_status"`
	ConsecutiveHealthFailures int32                `json:"consecutive_health_failures"`
	LastCheckedAt             *time.Time           `json:"last_checked_at,omitempty"`
	LastHealthCheckedAt       *time.Time           `json:"last_health_checked_at,omitempty"`
	LastHealthFailureAt       *time.Time           `json:"last_health_failure_at,omitempty"`
	VerifiedAt                *time.Time           `json:"verified_at,omitempty"`
	DisabledAt                *time.Time           `json:"disabled_at,omitempty"`
	CreatedBy                 *string              `json:"created_by,omitempty"`
	CreatedAt                 time.Time            `json:"created_at"`
	UpdatedAt                 time.Time            `json:"updated_at"`
}

type CreateRequest struct {
	Domain           string `json:"domain"`
	Region           string `json:"region"`
	CustomReturnPath string `json:"custom_return_path"`
}

type CreateResult struct {
	Domain       *SenderDomain
	Provisioning bool
}

type ProvisioningResponse struct {
	Status            string `json:"status"`
	Message           string `json:"message"`
	RetryAfterSeconds int    `json:"retry_after_seconds"`
}
