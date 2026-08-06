package backoffice

import (
	"context"
	"errors"
)

const ServiceName = "dugble-backoffice"

type Database interface {
	Ping(context.Context) error
}

// Service owns backoffice application capabilities independently from HTTP
// transport and process wiring.
type Service struct {
	database Database
}

func NewService(database Database) (*Service, error) {
	if database == nil {
		return nil, errors.New("backoffice database is required")
	}
	return &Service{database: database}, nil
}

func (service *Service) Ready(ctx context.Context) error {
	if service == nil || service.database == nil {
		return errors.New("backoffice service is not configured")
	}
	if ctx == nil {
		return errors.New("backoffice readiness context is required")
	}
	return service.database.Ping(ctx)
}
