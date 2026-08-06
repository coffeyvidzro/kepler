package sender

// OwnerType identifies who owns a canonical sender asset.
type OwnerType string

const (
	OwnerPlatform OwnerType = "platform"
	OwnerTeam     OwnerType = "team"
)

// AssetStatus is the lifecycle state of a sender asset.
type AssetStatus string

const (
	AssetStatusPending   AssetStatus = "pending"
	AssetStatusActive    AssetStatus = "active"
	AssetStatusDegraded  AssetStatus = "degraded"
	AssetStatusSuspended AssetStatus = "suspended"
	AssetStatusDisabled  AssetStatus = "disabled"
	AssetStatusFailed    AssetStatus = "failed"
)

// HealthStatus is the operational health of a sender asset or provider binding.
type HealthStatus string

const (
	HealthUnknown  HealthStatus = "unknown"
	HealthHealthy  HealthStatus = "healthy"
	HealthDegraded HealthStatus = "degraded"
)

// BindingStatus is the provider-registration lifecycle of a sender binding.
type BindingStatus string

const (
	BindingStatusPending   BindingStatus = "pending"
	BindingStatusActive    BindingStatus = "active"
	BindingStatusRejected  BindingStatus = "rejected"
	BindingStatusSuspended BindingStatus = "suspended"
	BindingStatusDisabled  BindingStatus = "disabled"
	BindingStatusFailed    BindingStatus = "failed"
	BindingStatusUnknown   BindingStatus = "unknown"
)

// GrantStatus identifies whether a team may use a sender asset.
type GrantStatus string

const (
	GrantStatusActive  GrantStatus = "active"
	GrantStatusRevoked GrantStatus = "revoked"
)

func (owner OwnerType) Valid() bool {
	switch owner {
	case OwnerPlatform, OwnerTeam:
		return true
	default:
		return false
	}
}

func (status AssetStatus) Valid() bool {
	switch status {
	case AssetStatusPending, AssetStatusActive, AssetStatusDegraded,
		AssetStatusSuspended, AssetStatusDisabled, AssetStatusFailed:
		return true
	default:
		return false
	}
}

func (status HealthStatus) Valid() bool {
	switch status {
	case HealthUnknown, HealthHealthy, HealthDegraded:
		return true
	default:
		return false
	}
}

func (status BindingStatus) Valid() bool {
	switch status {
	case BindingStatusPending, BindingStatusActive, BindingStatusRejected,
		BindingStatusSuspended, BindingStatusDisabled, BindingStatusFailed,
		BindingStatusUnknown:
		return true
	default:
		return false
	}
}

func (status GrantStatus) Valid() bool {
	return status == GrantStatusActive || status == GrantStatusRevoked
}
