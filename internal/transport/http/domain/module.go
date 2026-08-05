package domain

import domainmodule "github.com/coffeyvidzro/dugble/server/internal/modules/domain"

const (
	emailInfrastructureRetryAfterSeconds   = 10
	emailInfrastructureProvisioningMessage = "Customer email infrastructure is being prepared"
)

type Service = domainmodule.Service
type CreateRequest = domainmodule.CreateRequest
type ProvisioningResponse = domainmodule.ProvisioningResponse

var NewService = domainmodule.NewService
