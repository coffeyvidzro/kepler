package messagingrouting

import (
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging"
	platformrouting "github.com/coffeyvidzro/dugble/server/internal/platform/messaging/routing"
	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging/sender"
)

func scanCandidates(rows pgx.Rows) ([]platformrouting.Candidate, error) {
	defer rows.Close()
	candidates := make([]platformrouting.Candidate, 0)
	for rows.Next() {
		var candidate platformrouting.Candidate
		var revoked bool
		if err := rows.Scan(
			&candidate.Asset.ID, &candidate.Asset.Channel, &candidate.Asset.Identity,
			&candidate.Asset.NormalizedIdentity, &candidate.Asset.Status, &candidate.Asset.HealthStatus,
			&candidate.Grant.ID, &candidate.Grant.TeamID, &candidate.Grant.SenderAssetID,
			&candidate.Grant.Channel, &candidate.Grant.Status, &candidate.Grant.Default, &revoked,
			&candidate.Binding.ID, &candidate.Binding.SenderAssetID, &candidate.Binding.Provider,
			&candidate.Binding.ProviderAccount, &candidate.Binding.Region, &candidate.Binding.CountryCode,
			&candidate.Binding.Status, &candidate.Binding.Verified, &candidate.Binding.HealthStatus,
		); err != nil {
			return nil, fmt.Errorf("scan messaging route candidate: %w", err)
		}
		if revoked {
			revokedAt := time.Unix(0, 0).UTC()
			candidate.Grant.RevokedAt = &revokedAt
		}
		candidate.Capabilities = capabilitiesFor(candidate.Asset.Channel, candidate.Binding.Region, candidate.Binding.CountryCode)
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
