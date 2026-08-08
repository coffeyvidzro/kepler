package messagingrouting

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging"
	platformrouting "github.com/coffeyvidzro/dugble/server/internal/platform/messaging/routing"
	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging/sender"
)

type DBTX interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type Repository struct {
	db DBTX
}

func NewRepository(db DBTX) *Repository {
	return &Repository{db: db}
}

func Resolve(ctx context.Context, db DBTX, request platformrouting.Request) (platformrouting.Route, error) {
	resolver, err := platformrouting.NewResolver(NewRepository(db), platformrouting.DeterministicStrategy{})
	if err != nil {
		return platformrouting.Route{}, err
	}
	return resolver.Resolve(ctx, request)
}

func ResolveAll(ctx context.Context, db DBTX, request platformrouting.Request) ([]platformrouting.Route, error) {
	resolver, err := platformrouting.NewResolver(NewRepository(db), platformrouting.DeterministicStrategy{})
	if err != nil {
		return nil, err
	}
	return resolver.ResolveAll(ctx, request)
}

func (repository *Repository) ListCandidates(
	ctx context.Context,
	request platformrouting.Request,
) ([]platformrouting.Candidate, error) {
	if repository == nil || repository.db == nil {
		return nil, fmt.Errorf("messaging routing repository is not configured")
	}
	rows, err := repository.db.Query(ctx, `
		SELECT sender_id.id, 'sms', sender_id.name, sender_id.normalized_name,
			CASE sender_id.status WHEN 'approved' THEN 'active' ELSE sender_id.status END,
			sender_id.health_status,
			sender_id.id, sender_id.team_id, sender_id.id, 'sms',
			CASE WHEN sender_id.disabled_at IS NULL THEN 'active' ELSE 'revoked' END,
			false, (sender_id.disabled_at IS NOT NULL),
			sender_id.id, sender_id.id, COALESCE(sender_id.provider, ''),
			COALESCE(sender_id.provider, ''), '', sender_id.country_code::text,
			CASE sender_id.status WHEN 'approved' THEN 'active' ELSE sender_id.status END,
			sender_id.provider_whitelisted, sender_id.health_status
		FROM sender_ids AS sender_id
		WHERE sender_id.team_id = $1
		  AND $2 = 'sms'
		  AND sender_id.provider IS NOT NULL
		  AND sender_id.disabled_at IS NULL
		ORDER BY sender_id.provider, sender_id.country_code, sender_id.id
		FOR SHARE OF sender_id
	`, request.TeamID, string(request.Channel))
	if err != nil {
		return nil, fmt.Errorf("query messaging route candidates: %w", err)
	}
	defer rows.Close()

	candidates := make([]platformrouting.Candidate, 0)
	for rows.Next() {
		var candidate platformrouting.Candidate
		var revoked bool
		if err := rows.Scan(
			&candidate.Asset.ID,
			&candidate.Asset.Channel,
			&candidate.Asset.Identity,
			&candidate.Asset.NormalizedIdentity,
			&candidate.Asset.Status,
			&candidate.Asset.HealthStatus,
			&candidate.Grant.ID,
			&candidate.Grant.TeamID,
			&candidate.Grant.SenderAssetID,
			&candidate.Grant.Channel,
			&candidate.Grant.Status,
			&candidate.Grant.Default,
			&revoked,
			&candidate.Binding.ID,
			&candidate.Binding.SenderAssetID,
			&candidate.Binding.Provider,
			&candidate.Binding.ProviderAccount,
			&candidate.Binding.Region,
			&candidate.Binding.CountryCode,
			&candidate.Binding.Status,
			&candidate.Binding.Verified,
			&candidate.Binding.HealthStatus,
		); err != nil {
			return nil, fmt.Errorf("scan messaging route candidate: %w", err)
		}
		if revoked {
			revokedAt := time.Unix(0, 0).UTC()
			candidate.Grant.RevokedAt = &revokedAt
		}
		candidate.Capabilities = capabilitiesFor(
			candidate.Asset.Channel,
			candidate.Binding.Region,
			candidate.Binding.CountryCode,
		)
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messaging route candidates: %w", err)
	}
	return candidates, nil
}

func capabilitiesFor(channel messaging.Channel, region string, country string) sender.CapabilitySet {
	capabilities := make(sender.CapabilitySet)
	switch channel {
	case messaging.ChannelEmail:
		capabilities[sender.CapabilityDomainVerification] = struct{}{}
		capabilities[sender.CapabilityPushDeliveryFeedback] = struct{}{}
	case messaging.ChannelSMS:
		capabilities[sender.CapabilitySenderIDRegistration] = struct{}{}
		capabilities[sender.CapabilityPollDeliveryFeedback] = struct{}{}
	}
	if strings.TrimSpace(region) != "" || strings.TrimSpace(country) != "" {
		capabilities[sender.CapabilityGeographicRouting] = struct{}{}
	}
	return capabilities
}
