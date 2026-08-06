package teams

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidRequest = errors.New("invalid team request")

type Service struct {
	repository *Repository
}

func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) List(ctx context.Context, filter Filter) ([]Row, error) {
	filter.Query = strings.TrimSpace(filter.Query)
	return s.repository.List(ctx, filter)
}

func (s *Service) Detail(ctx context.Context, id string) (Detail, error) {
	return s.repository.Detail(ctx, strings.TrimSpace(id))
}

func (s *Service) UpdateStatus(ctx context.Context, id string, req StatusRequest) error {
	var status string
	switch strings.TrimSpace(req.Action) {
	case "enable":
		status = "active"
	case "disable":
		status = "disabled"
	default:
		return fmt.Errorf("%w: action must be enable or disable", ErrInvalidRequest)
	}

	if strings.TrimSpace(req.Reason) == "" {
		return fmt.Errorf("%w: status change reason is required", ErrInvalidRequest)
	}

	return s.repository.UpdateStatus(ctx, strings.TrimSpace(id), status)
}
