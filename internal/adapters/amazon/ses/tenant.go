package ses

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	sestypes "github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/aws/smithy-go"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"
)

const (
	TransactionalConfigurationSet = "dugble-transactional"
	MarketingConfigurationSet     = "dugble-marketing"
)

type TenantProvisionRequest struct {
	Region           string
	ExternalName     string
	SuppressionScope string
	ReputationPolicy string
}

type TenantProvisionResult struct {
	ExternalID string
	TenantARN  string
}

// ProvisionTenant converges an SES tenant and its shared resource associations
// on Dugble's desired state. Every operation is safe to repeat after a partial
// failure or a redelivered JetStream command.
func (c *Client) ProvisionTenant(ctx context.Context, request TenantProvisionRequest) (TenantProvisionResult, error) {
	client, err := c.tenantClient(request.Region)
	if err != nil {
		return TenantProvisionResult{}, err
	}
	name := strings.TrimSpace(request.ExternalName)
	if name == "" {
		return TenantProvisionResult{}, errors.New("SES tenant name is required")
	}

	scope, err := suppressionScope(request.SuppressionScope)
	if err != nil {
		return TenantProvisionResult{}, err
	}
	suppression := &sestypes.TenantSuppressionAttributes{
		SuppressionScope: scope,
		SuppressedReasons: []sestypes.SuppressionListReason{
			sestypes.SuppressionListReasonBounce,
			sestypes.SuppressionListReasonComplaint,
		},
	}

	tenant, err := createOrGetTenant(ctx, client, name, suppression)
	if err != nil {
		return TenantProvisionResult{}, err
	}
	if tenant.TenantId == nil || strings.TrimSpace(*tenant.TenantId) == "" || tenant.TenantArn == nil || strings.TrimSpace(*tenant.TenantArn) == "" {
		return TenantProvisionResult{}, errors.New("SES returned incomplete tenant identity")
	}

	_, err = client.PutTenantSuppressionAttributes(ctx, &sesv2.PutTenantSuppressionAttributesInput{
		TenantName:        aws.String(name),
		SuppressionScope:  scope,
		SuppressedReasons: suppression.SuppressedReasons,
	})
	if err != nil {
		return TenantProvisionResult{}, fmt.Errorf("configure SES tenant suppression: %w", err)
	}

	tenantARN := strings.TrimSpace(*tenant.TenantArn)
	resources := []struct {
		label string
		arn   string
	}{}
	for _, configurationSet := range []string{TransactionalConfigurationSet, MarketingConfigurationSet} {
		resourceARN, arnErr := configurationSetARN(tenantARN, configurationSet)
		if arnErr != nil {
			return TenantProvisionResult{}, arnErr
		}
		resources = append(resources, struct {
			label string
			arn   string
		}{label: "configuration set " + configurationSet, arn: resourceARN})
	}
	onboardingARN, err := identityARN(tenantARN, platformemail.CustomerOnboardingSESIdentity)
	if err != nil {
		return TenantProvisionResult{}, err
	}
	resources = append(resources, struct {
		label string
		arn   string
	}{label: "onboarding domain identity", arn: onboardingARN})

	for _, resource := range resources {
		_, associationErr := client.CreateTenantResourceAssociation(ctx, &sesv2.CreateTenantResourceAssociationInput{
			TenantName:  aws.String(name),
			ResourceArn: aws.String(resource.arn),
		})
		if associationErr != nil && !isAlreadyExists(associationErr) {
			return TenantProvisionResult{}, fmt.Errorf("associate SES %s: %w", resource.label, associationErr)
		}
	}

	policyARN, err := reputationPolicyARN(tenantARN, request.ReputationPolicy)
	if err != nil {
		return TenantProvisionResult{}, err
	}
	_, err = client.UpdateReputationEntityPolicy(ctx, &sesv2.UpdateReputationEntityPolicyInput{
		ReputationEntityType:      sestypes.ReputationEntityTypeResource,
		ReputationEntityReference: aws.String(tenantARN),
		ReputationEntityPolicy:    aws.String(policyARN),
	})
	if err != nil {
		return TenantProvisionResult{}, fmt.Errorf("apply SES tenant reputation policy: %w", err)
	}

	return TenantProvisionResult{ExternalID: strings.TrimSpace(*tenant.TenantId), TenantARN: tenantARN}, nil
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

func isAlreadyExists(err error) bool {
	var apiError smithy.APIError
	return errors.As(err, &apiError) && strings.EqualFold(apiError.ErrorCode(), "AlreadyExistsException")
}
