package domain

import (
	"time"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/awsses"
)

const (
	DefaultProvider         = "aws_ses"
	DefaultProviderAccount  = "default"
	DefaultCustomReturnPath = "send"
	DefaultTLSMode          = "opportunistic"

	StatusNotStarted        = "not_started"
	StatusPending           = "pending"
	StatusVerified          = "verified"
	StatusPartiallyVerified = "partially_verified"
	StatusPartiallyFailed   = "partially_failed"
	StatusFailed            = "failed"
	StatusTemporaryFailure  = "temporary_failure"
	StatusDisabled          = "disabled"

	HealthStatusUnknown  = "unknown"
	HealthStatusHealthy  = "healthy"
	HealthStatusDegraded = "degraded"

	ClaimStatusPending    = "pending"
	ClaimStatusVerified   = "verified"
	ClaimStatusCompleted  = "completed"
	ClaimStatusBlocked    = "blocked"
	ClaimStatusExpired    = "expired"
	ClaimStatusSuperseded = "superseded"
	ClaimStatusCanceled   = "canceled"
	ClaimStatusFailed     = "failed"

	ClaimBlockedGracePeriod            = "grace_period"
	ClaimBlockedRecentOwnerActivity    = "recent_owner_activity"
	ClaimBlockedPendingScheduledEmails = "pending_scheduled_emails"

	DefaultHealthFailureThreshold int32 = 3
	DefaultClaimLifetime                = 7 * 24 * time.Hour
)

type VerificationRecord = platformemail.VerificationRecord

type Capabilities struct {
	Sending   bool `json:"sending"`
	Receiving bool `json:"receiving"`
}

type SenderDomain struct {
	ID                        string               `json:"id"`
	TeamID                    string               `json:"team_id"`
	Domain                    string               `json:"name"`
	Provider                  string               `json:"provider,omitempty"`
	ProviderAccount           string               `json:"provider_account,omitempty"`
	ProviderRegion            string               `json:"region"`
	ProviderExternalID        *string              `json:"provider_external_id,omitempty"`
	Status                    string               `json:"status"`
	ProviderStatus            *string              `json:"provider_status,omitempty"`
	VerificationRecords       []VerificationRecord `json:"records"`
	OpenTracking              bool                 `json:"open_tracking"`
	ClickTracking             bool                 `json:"click_tracking"`
	TrackingSubdomain         *string              `json:"tracking_subdomain,omitempty"`
	ActiveTrackingSubdomain   *string              `json:"active_tracking_subdomain,omitempty"`
	TLS                       string               `json:"tls"`
	Capabilities              Capabilities         `json:"capabilities"`
	CustomReturnPath          string               `json:"custom_return_path"`
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
	Name              string        `json:"name"`
	Domain            string        `json:"domain,omitempty"`
	Region            string        `json:"region"`
	CustomReturnPath  string        `json:"custom_return_path"`
	OpenTracking      *bool         `json:"open_tracking,omitempty"`
	ClickTracking     *bool         `json:"click_tracking,omitempty"`
	TrackingSubdomain *string       `json:"tracking_subdomain,omitempty"`
	TLS               string        `json:"tls,omitempty"`
	Capabilities      *Capabilities `json:"capabilities,omitempty"`
}

type UpdateRequest struct {
	OpenTracking      *bool         `json:"open_tracking,omitempty"`
	ClickTracking     *bool         `json:"click_tracking,omitempty"`
	TrackingSubdomain *string       `json:"tracking_subdomain,omitempty"`
	TLS               *string       `json:"tls,omitempty"`
	Capabilities      *Capabilities `json:"capabilities,omitempty"`
}

type DomainConfiguration struct {
	OpenTracking      bool
	ClickTracking     bool
	TrackingSubdomain *string
	TLS               string
	Capabilities      Capabilities
	CustomReturnPath  string
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

type ClaimRequest struct {
	Name              string        `json:"name"`
	Region            string        `json:"region"`
	CustomReturnPath  string        `json:"custom_return_path"`
	OpenTracking      *bool         `json:"open_tracking,omitempty"`
	ClickTracking     *bool         `json:"click_tracking,omitempty"`
	TrackingSubdomain *string       `json:"tracking_subdomain,omitempty"`
	TLS               string        `json:"tls,omitempty"`
	Capabilities      *Capabilities `json:"capabilities,omitempty"`
}

type DomainClaim struct {
	ID                      string             `json:"id"`
	DomainID                string             `json:"domain_id"`
	Name                    string             `json:"name"`
	Status                  string             `json:"status"`
	SourceDomainID          *string            `json:"source_domain_id,omitempty"`
	SourceTeamID            string             `json:"source_team_id"`
	TargetTeamID            string             `json:"target_team_id"`
	Region                  string             `json:"region"`
	CustomReturnPath        string             `json:"custom_return_path"`
	OpenTracking            bool               `json:"open_tracking"`
	ClickTracking           bool               `json:"click_tracking"`
	TrackingSubdomain       *string            `json:"tracking_subdomain,omitempty"`
	TLS                     string             `json:"tls"`
	Capabilities            Capabilities       `json:"capabilities"`
	VerificationRecord      VerificationRecord `json:"record"`
	BlockedReason           *string            `json:"blocked_reason,omitempty"`
	FailureReason           *string            `json:"failure_reason,omitempty"`
	VerificationRequestedAt *time.Time         `json:"verification_requested_at,omitempty"`
	VerifiedAt              *time.Time         `json:"verified_at,omitempty"`
	CompletedAt             *time.Time         `json:"completed_at,omitempty"`
	ExpiresAt               time.Time          `json:"expires_at"`
	CreatedAt               time.Time          `json:"created_at"`
	UpdatedAt               time.Time          `json:"updated_at"`
}
