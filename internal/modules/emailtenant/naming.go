package emailtenant

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const awsTenantNamePrefix = "dugble-t-"

// AWSExternalName derives a stable, opaque SES tenant name from the immutable
// Dugble team ID. Team names and domains are deliberately excluded because they
// can change during the lifetime of the provider tenant.
func AWSExternalName(teamID uuid.UUID) string {
	return awsTenantNamePrefix + strings.ReplaceAll(teamID.String(), "-", "")
}

func ParseTeamID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse email tenant team id: %w", err)
	}
	return id, nil
}
