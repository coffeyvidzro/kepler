// Package email preserves the former platform email import path while the
// AWS SES-specific contract and policy move to internal/platform/awsses.
//
// Deprecated: use internal/platform/awsses.
package email

import awsses "github.com/coffeyvidzro/dugble/server/internal/platform/awsses"

const (
	RecordDKIM = awsses.RecordDKIM
	RecordSPF  = awsses.RecordSPF

	RecordTypeTXT = awsses.RecordTypeTXT
	RecordTypeMX  = awsses.RecordTypeMX

	RecordStatusPending  = awsses.RecordStatusPending
	RecordStatusVerified = awsses.RecordStatusVerified
	RecordStatusFailed   = awsses.RecordStatusFailed

	SystemSESTenantName           = awsses.SystemSESTenantName
	CustomerSandboxSESTenantName  = awsses.CustomerSandboxSESTenantName
	CustomerOnboardingIdentity    = awsses.CustomerOnboardingIdentity
	CustomerOnboardingSESIdentity = awsses.CustomerOnboardingSESIdentity

	MaxRawMessageBytes           = awsses.MaxRawMessageBytes
	MaxBodyBytes                 = awsses.MaxBodyBytes
	MaxAttachmentsDecodedBytes   = awsses.MaxAttachmentsDecodedBytes
	MaxBatchPayloadBytes         = awsses.MaxBatchPayloadBytes
	MaxHTTPRequestBytes          = awsses.MaxHTTPRequestBytes
)

type Address = awsses.Address
type Attachment = awsses.Attachment
type Message = awsses.Message
type Result = awsses.Result
type Sender = awsses.Sender
type VerificationRecord = awsses.VerificationRecord
type DomainProvisionRequest = awsses.DomainProvisionRequest
type DomainStatus = awsses.DomainStatus
type DomainProvider = awsses.DomainProvider
type SendError = awsses.SendError
type DNSVerifier = awsses.DNSVerifier
type DeliveryRoute = awsses.DeliveryRoute

var (
	NewSendError              = awsses.NewSendError
	NewSubmissionUnknownError = awsses.NewSubmissionUnknownError
	IsSubmissionUnknown       = awsses.IsSubmissionUnknown
	IsRetryable               = awsses.IsRetryable
	FailureCode               = awsses.FailureCode

	SystemDeliveryRoute          = awsses.SystemDeliveryRoute
	CustomerSandboxDeliveryRoute = awsses.CustomerSandboxDeliveryRoute
	CustomerDeliveryRoute        = awsses.CustomerDeliveryRoute
	BuiltInDeliveryRoute         = awsses.BuiltInDeliveryRoute
	PersistDeliveryRoute         = awsses.PersistDeliveryRoute
	ExtractDeliveryRoute         = awsses.ExtractDeliveryRoute

	NormalizeSESRegion  = awsses.NormalizeSESRegion
	SupportedSESRegions = awsses.SupportedSESRegions

	ErrSandboxTeamEmailNotVerified = awsses.ErrSandboxTeamEmailNotVerified
	ErrSandboxRecipientRestricted  = awsses.ErrSandboxRecipientRestricted
	ValidateSandboxRecipient       = awsses.ValidateSandboxRecipient
)
