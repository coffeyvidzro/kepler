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
	messageIDTagName = "dugble_message_id"
	attemptIDTagName = "dugble_attempt_id"
	streamTagName    = "dugble_stream"
)

func (c *Client) Send(ctx context.Context, message platformemail.Message) (platformemail.Result, error) {
	region, supported := platformemail.NormalizeSESRegion(message.Region)
	if !supported {
		return platformemail.Result{}, platformemail.NewSendError(
			"invalid_region",
			false,
			fmt.Errorf("SES delivery region %q is not supported", strings.TrimSpace(message.Region)),
		)
	}
	message.Region = region
	client, err := c.v2SendingClient(message.Region)
	if err != nil {
		return platformemail.Result{}, err
	}
	if strings.TrimSpace(message.From.Email) == "" {
		message.From.Email = c.defaultFrom
	}
	raw, err := buildMIME(message)
	if err != nil {
		code := "invalid_message"
		switch {
		case errors.Is(err, ErrUnsupportedAttachmentPath):
			code = "unsupported_attachment_path"
		case errors.Is(err, ErrMessageTooLarge):
			code = "message_too_large"
		case errors.Is(err, ErrReservedHeader):
			code = "reserved_header"
		}
		return platformemail.Result{}, platformemail.NewSendError(code, false, err)
	}

	configurationSet := strings.TrimSpace(message.ConfigurationSet)
	if configurationSet == "" {
		return platformemail.Result{}, platformemail.NewSendError(
			"missing_configuration_set",
			false,
			fmt.Errorf("SES configuration set is required for %s email stream", strings.TrimSpace(message.Stream)),
		)
	}
	tenantName := strings.TrimSpace(message.SESTenantName)
	if tenantName == "" {
		return platformemail.Result{}, platformemail.NewSendError(
			"missing_ses_tenant",
			false,
			errors.New("SES tenant name is required"),
		)
	}

	input := &sesv2.SendEmailInput{
		ConfigurationSetName: aws.String(configurationSet),
		TenantName:           aws.String(tenantName),
		FromEmailAddress:     aws.String(formatAddress(message.From)),
		Destination:          destination(message),
		Content: &sestypes.EmailContent{
			Raw: &sestypes.RawMessage{Data: raw},
		},
		EmailTags: deliveryTags(message),
	}
	output, err := client.SendEmail(ctx, input)
	if err != nil {
		return platformemail.Result{}, classifySESFailure(err)
	}
	if output.MessageId == nil || strings.TrimSpace(*output.MessageId) == "" {
		return platformemail.Result{}, platformemail.NewSubmissionUnknownError(
			"empty_provider_message_id",
			errors.New("SES returned an empty message ID after accepting the request"),
		)
	}
	return platformemail.Result{Provider: ProviderSES, MessageID: strings.TrimSpace(*output.MessageId)}, nil
}

func deliveryTags(message platformemail.Message) []sestypes.MessageTag {
	tags := make([]sestypes.MessageTag, 0, 3)
	if value := strings.TrimSpace(message.MessageID); value != "" {
		tags = append(tags, sestypes.MessageTag{Name: aws.String(messageIDTagName), Value: aws.String(value)})
	}
	if value := strings.TrimSpace(message.AttemptID); value != "" {
		tags = append(tags, sestypes.MessageTag{Name: aws.String(attemptIDTagName), Value: aws.String(value)})
	}
	if value := strings.TrimSpace(message.Stream); value != "" {
		tags = append(tags, sestypes.MessageTag{Name: aws.String(streamTagName), Value: aws.String(value)})
	}
	return tags
}

func destination(message platformemail.Message) *sestypes.Destination {
	return &sestypes.Destination{
		ToAddresses:  addressValues(message.To),
		CcAddresses:  addressValues(message.CC),
		BccAddresses: addressValues(message.BCC),
	}
}

func addressValues(addresses []platformemail.Address) []string {
	values := make([]string, 0, len(addresses))
	seen := make(map[string]struct{}, len(addresses))
	for _, address := range addresses {
		email := strings.TrimSpace(address.Email)
		key := strings.ToLower(email)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		values = append(values, email)
	}
	return values
}

func classifySESFailure(err error) error {
	var apiError smithy.APIError
	if !errors.As(err, &apiError) {
		return platformemail.NewSubmissionUnknownError("ses_submission_unknown", err)
	}
	code := strings.ToLower(strings.TrimSpace(apiError.ErrorCode()))
	retryable := false
	switch code {
	case "throttling", "throttlingexception", "serviceunavailable", "internalfailure", "internalservererror":
		retryable = true
	case "requesttimeout", "requesttimeoutexception":
		return platformemail.NewSubmissionUnknownError(code, err)
	}
	return platformemail.NewSendError(code, retryable, err)
}
