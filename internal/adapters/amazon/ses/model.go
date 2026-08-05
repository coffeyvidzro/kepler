package ses

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	sestypes "github.com/aws/aws-sdk-go-v2/service/sesv2/types"

	"github.com/coffeyvidzro/dugble/server/internal/modules/emailtenant"
)

const (
	ProviderSES                   = "ses"
	TransactionalConfigurationSet = "dugble-transactional"
	MarketingConfigurationSet     = "dugble-marketing"
)

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

// ProvisionTenant converges an SES tenant and its shared resource associations
// on Dugble's desired state. Every operation is safe to repeat after a partial
// failure or a redelivered command.
func (client *Client) ProvisionTenant(ctx context.Context, request emailtenant.ProvisionRequest) (emailtenant.ProvisionResult, error) {
	tenantClient, err := client.tenantClient(request.Region)
	if err != nil {
		return emailtenant.ProvisionResult{}, err
	}
	name := strings.TrimSpace(request.ExternalName)
	if name == "" {
		return emailtenant.ProvisionResult{}, errors.New("SES tenant name is required")
	}

	scope, err := suppressionScope(request.SuppressionScope)
	if err != nil {
		return emailtenant.ProvisionResult{}, err
	}
	suppression := &sestypes.TenantSuppressionAttributes{
		SuppressionScope: scope,
		SuppressedReasons: []sestypes.SuppressionListReason{
			sestypes.SuppressionListReasonBounce,
			sestypes.SuppressionListReasonComplaint,
		},
	}

	tenant, err := createOrGetTenant(ctx, tenantClient, name, suppression)
	if err != nil {
		return emailtenant.ProvisionResult{}, err
	}
	if tenant.TenantId == nil || strings.TrimSpace(*tenant.TenantId) == "" || tenant.TenantArn == nil || strings.TrimSpace(*tenant.TenantArn) == "" {
		return emailtenant.ProvisionResult{}, errors.New("SES returned incomplete tenant identity")
	}

	_, err = tenantClient.PutTenantSuppressionAttributes(ctx, &sesv2.PutTenantSuppressionAttributesInput{
		TenantName:        aws.String(name),
		SuppressionScope:  scope,
		SuppressedReasons: suppression.SuppressedReasons,
	})
	if err != nil {
		return emailtenant.ProvisionResult{}, fmt.Errorf("configure SES tenant suppression: %w", err)
	}

	tenantARN := strings.TrimSpace(*tenant.TenantArn)
	resources := []struct {
		label string
		arn   string
	}{}
	for _, configurationSet := range []string{TransactionalConfigurationSet, MarketingConfigurationSet} {
		resourceARN, arnErr := configurationSetARN(tenantARN, configurationSet)
		if arnErr != nil {
			return emailtenant.ProvisionResult{}, arnErr
		}
		resources = append(resources, struct {
			label string
			arn   string
		}{label: "configuration set " + configurationSet, arn: resourceARN})
	}
	for _, resource := range resources {
		_, associationErr := tenantClient.CreateTenantResourceAssociation(ctx, &sesv2.CreateTenantResourceAssociationInput{
			TenantName:  aws.String(name),
			ResourceArn: aws.String(resource.arn),
		})
		if associationErr != nil && !isAlreadyExists(associationErr) {
			return emailtenant.ProvisionResult{}, fmt.Errorf("associate SES %s: %w", resource.label, associationErr)
		}
	}

	policyARN, err := reputationPolicyARN(tenantARN, request.ReputationPolicy)
	if err != nil {
		return emailtenant.ProvisionResult{}, err
	}
	_, err = tenantClient.UpdateReputationEntityPolicy(ctx, &sesv2.UpdateReputationEntityPolicyInput{
		ReputationEntityType:      sestypes.ReputationEntityTypeResource,
		ReputationEntityReference: aws.String(tenantARN),
		ReputationEntityPolicy:    aws.String(policyARN),
	})
	if err != nil {
		return emailtenant.ProvisionResult{}, fmt.Errorf("apply SES tenant reputation policy: %w", err)
	}

	return emailtenant.ProvisionResult{
		ExternalID: strings.TrimSpace(*tenant.TenantId),
		TenantARN:  tenantARN,
	}, nil
}

func createOrGetTenant(ctx context.Context, client sesTenantAPI, name string, suppression *sestypes.TenantSuppressionAttributes) (*sestypes.Tenant, error) {
	output, err := client.CreateTenant(ctx, &sesv2.CreateTenantInput{
		TenantName:            aws.String(name),
		SuppressionAttributes: suppression,
	})
	if err == nil {
		return &sestypes.Tenant{
			TenantId:              output.TenantId,
			TenantArn:             output.TenantArn,
			TenantName:            output.TenantName,
			SuppressionAttributes: output.SuppressionAttributes,
		}, nil
	}
	if !isAlreadyExists(err) {
		return nil, fmt.Errorf("create SES tenant: %w", err)
	}
	getOutput, getErr := client.GetTenant(ctx, &sesv2.GetTenantInput{TenantName: aws.String(name)})
	if getErr != nil {
		return nil, fmt.Errorf("get existing SES tenant: %w", getErr)
	}
	if getOutput.Tenant == nil {
		return nil, errors.New("SES returned an empty existing tenant")
	}
	return getOutput.Tenant, nil
}

func suppressionScope(value string) (sestypes.SuppressionListScope, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "tenant", "":
		return sestypes.SuppressionListScopeTenant, nil
	case "account":
		return sestypes.SuppressionListScopeAccount, nil
	default:
		return "", fmt.Errorf("unsupported SES suppression scope %q", value)
	}
}

func configurationSetARN(tenantARN, configurationSet string) (string, error) {
	parts := strings.Split(tenantARN, ":")
	if len(parts) < 6 || parts[0] != "arn" || parts[2] != "ses" || strings.TrimSpace(parts[3]) == "" || strings.TrimSpace(parts[4]) == "" {
		return "", fmt.Errorf("invalid SES tenant ARN %q", tenantARN)
	}
	return fmt.Sprintf("arn:%s:ses:%s:%s:configuration-set/%s", parts[1], parts[3], parts[4], configurationSet), nil
}

func reputationPolicyARN(tenantARN, policy string) (string, error) {
	policy = strings.ToLower(strings.TrimSpace(policy))
	if policy == "" {
		policy = "standard"
	}
	switch policy {
	case "none", "standard", "strict":
	default:
		return "", fmt.Errorf("unsupported SES reputation policy %q", policy)
	}
	parts := strings.Split(tenantARN, ":")
	if len(parts) < 6 || parts[0] != "arn" || parts[2] != "ses" || strings.TrimSpace(parts[3]) == "" {
		return "", fmt.Errorf("invalid SES tenant ARN %q", tenantARN)
	}
	return fmt.Sprintf("arn:%s:ses:%s:aws:reputation-policy/%s", parts[1], parts[3], policy), nil
}
