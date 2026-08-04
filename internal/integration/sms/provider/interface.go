// Package provider contains the common upstream-provider contract used by the
// routing package and concrete SMS adapters.
package provider

import "github.com/coffeyvidzro/dugble/server/internal/integration/sms"

// Provider is an alias of sms.Provider.
//
// The canonical interface must live in the parent sms package because concrete
// adapters already depend on sms.SendRequest, sms.SendResponse, and
// sms.StatusResponse. Defining a second canonical interface here and importing
// it from sms.Service would create this cycle:
//
//	sms -> provider -> sms
//
// The alias keeps the readable provider.Provider name for routing code without
// introducing that cycle.
type Provider = sms.Provider
