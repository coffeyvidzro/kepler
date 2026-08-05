// Package tenantprovision preserves the former import path while email tenant
// provisioning moves under the emailtenant feature.
//
// Deprecated: use internal/modules/emailtenant/provisioning.
package tenantprovision

import provisioning "github.com/coffeyvidzro/dugble/server/internal/modules/emailtenant/provisioning"

const (
	ProvisionSubject   = provisioning.ProvisionSubject
	ProvisionEventType = provisioning.ProvisionEventType
	ConsumerName       = provisioning.ConsumerName
	DLQSubject         = provisioning.DLQSubject
)

type Command = provisioning.Command
type Config = provisioning.Config
type Consumer = provisioning.Consumer
type Queue = provisioning.Queue
type Processor = provisioning.Processor
type Handler = provisioning.Handler

var (
	ErrQueueNotConfigured     = provisioning.ErrQueueNotConfigured
	ErrConsumerNotConfigured  = provisioning.ErrConsumerNotConfigured
	ErrProcessorNotConfigured = provisioning.ErrProcessorNotConfigured
	ErrTransactionRequired    = provisioning.ErrTransactionRequired

	DefaultRetryBackOff = provisioning.DefaultRetryBackOff
	ValidateCommand     = provisioning.ValidateCommand
	NewQueue            = provisioning.NewQueue
	NewProcessor        = provisioning.NewProcessor
	NewHandler          = provisioning.NewHandler
	NewConsumer         = provisioning.NewConsumer
)
