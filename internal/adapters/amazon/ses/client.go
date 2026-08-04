package ses

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"
)

func NewClient(region, defaultFrom, accessKey, secretKey string, _ ...string) (*Client, error) {
	return newClient(context.Background(), region, defaultFrom, accessKey, secretKey)
}

func NewSESSender(ctx context.Context, region, defaultFrom, accessKey, secretKey string, _ ...string) (*Client, error) {
	return newClient(ctx, region, defaultFrom, accessKey, secretKey)
}

func newClient(ctx context.Context, region, defaultFrom, accessKey, secretKey string) (*Client, error) {
	region = strings.TrimSpace(region)
	accessKey = strings.TrimSpace(accessKey)
	secretKey = strings.TrimSpace(secretKey)
	if region == "" {
		return nil, errors.New("SES region is required")
	}
	if (accessKey == "") != (secretKey == "") {
		return nil, errors.New("AWS access key and secret key must be configured together")
	}

	options := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(region)}
	if accessKey != "" {
		options = append(options, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		))
	}
	resolved, err := awsconfig.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration: %w", err)
	}
	return &Client{
		defaultRegion:    region,
		defaultFrom:      strings.TrimSpace(defaultFrom),
		awsConfig:        resolved,
		v2SendingClients: make(map[string]sesV2SendAPI),
		identityClients:  make(map[string]sesIdentityAPI),
		tenantClients:    make(map[string]sesTenantAPI),
	}, nil
}

func (c *Client) regionalConfig(region string) aws.Config {
	config := c.awsConfig
	config.Region = strings.TrimSpace(region)
	return config
}

func (c *Client) v2SendingClient(region string) (sesV2SendAPI, error) {
	if c == nil {
		return nil, errors.New("SES client is not configured")
	}
	region = strings.TrimSpace(region)
	if region == "" {
		return nil, errors.New("SES delivery region is required")
	}
	if _, supported := platformemail.NormalizeSESRegion(region); !supported {
		return nil, fmt.Errorf("SES delivery region %q is not supported", region)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if client, ok := c.v2SendingClients[region]; ok {
		return client, nil
	}
	client := sesv2.NewFromConfig(c.regionalConfig(region))
	c.v2SendingClients[region] = client
	return client, nil
}

func (c *Client) identityClient(region string) (sesIdentityAPI, error) {
	if c == nil {
		return nil, errors.New("SES client is not configured")
	}
	region = strings.TrimSpace(region)
	if region == "" {
		region = c.defaultRegion
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if client, ok := c.identityClients[region]; ok {
		return client, nil
	}
	client := sesv2.NewFromConfig(c.regionalConfig(region))
	c.identityClients[region] = client
	return client, nil
}

func (c *Client) tenantClient(region string) (sesTenantAPI, error) {
	if c == nil {
		return nil, errors.New("SES client is not configured")
	}
	region = strings.TrimSpace(region)
	if region == "" {
		region = c.defaultRegion
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if client, ok := c.tenantClients[region]; ok {
		return client, nil
	}
	client := sesv2.NewFromConfig(c.regionalConfig(region))
	c.tenantClients[region] = client
	return client, nil
}
