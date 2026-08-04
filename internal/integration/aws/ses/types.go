package ses

import (
	"context"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
)

const ProviderSES = "ses"

type sesV2SendAPI interface {
	SendEmail(context.Context, *sesv2.SendEmailInput, ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error)
}

type sesIdentityAPI interface {
	CreateEmailIdentity(context.Context, *sesv2.CreateEmailIdentityInput, ...func(*sesv2.Options)) (*sesv2.CreateEmailIdentityOutput, error)
	PutEmailIdentityMailFromAttributes(context.Context, *sesv2.PutEmailIdentityMailFromAttributesInput, ...func(*sesv2.Options)) (*sesv2.PutEmailIdentityMailFromAttributesOutput, error)
	GetEmailIdentity(context.Context, *sesv2.GetEmailIdentityInput, ...func(*sesv2.Options)) (*sesv2.GetEmailIdentityOutput, error)
	DeleteEmailIdentity(context.Context, *sesv2.DeleteEmailIdentityInput, ...func(*sesv2.Options)) (*sesv2.DeleteEmailIdentityOutput, error)
}

type sesTenantAPI interface {
	CreateTenant(context.Context, *sesv2.CreateTenantInput, ...func(*sesv2.Options)) (*sesv2.CreateTenantOutput, error)
	GetTenant(context.Context, *sesv2.GetTenantInput, ...func(*sesv2.Options)) (*sesv2.GetTenantOutput, error)
	PutTenantSuppressionAttributes(context.Context, *sesv2.PutTenantSuppressionAttributesInput, ...func(*sesv2.Options)) (*sesv2.PutTenantSuppressionAttributesOutput, error)
	CreateTenantResourceAssociation(context.Context, *sesv2.CreateTenantResourceAssociationInput, ...func(*sesv2.Options)) (*sesv2.CreateTenantResourceAssociationOutput, error)
	UpdateReputationEntityPolicy(context.Context, *sesv2.UpdateReputationEntityPolicyInput, ...func(*sesv2.Options)) (*sesv2.UpdateReputationEntityPolicyOutput, error)
}

// Client implements provider-neutral sending, sender-domain, and tenant
// operations using AWS SES.
type Client struct {
	defaultRegion string
	defaultFrom   string
	awsConfig     aws.Config

	mu               sync.Mutex
	v2SendingClients map[string]sesV2SendAPI
	identityClients  map[string]sesIdentityAPI
	tenantClients    map[string]sesTenantAPI
}
