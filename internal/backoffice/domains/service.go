package domains

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidRequest = errors.New("invalid domain request")

type Service struct {
	repository *Repository
}

func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) List(ctx context.Context, filter Filter) ([]Row, error) {
	filter.Query = strings.TrimSpace(filter.Query)
	filter.Status = strings.TrimSpace(filter.Status)
	return s.repository.List(ctx, filter)
}

func (s *Service) Detail(ctx context.Context, id string) (Detail, error) {
	return s.repository.Detail(ctx, strings.TrimSpace(id))
}

func (s *Service) UpdateStatus(ctx context.Context, id string, req StatusRequest) error {
	switch strings.TrimSpace(req.Action) {
	case "verify":
		return s.repository.Verify(ctx, strings.TrimSpace(id))
	case "fail":
		reason := strings.TrimSpace(req.Reason)
		if reason == "" {
			return fmt.Errorf("%w: failure reason is required", ErrInvalidRequest)
		}
		return s.repository.Fail(ctx, strings.TrimSpace(id), reason)
	default:
		return fmt.Errorf("%w: action must be verify or fail", ErrInvalidRequest)
	}
}
