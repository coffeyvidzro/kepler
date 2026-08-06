package users

import (
	"context"
	"strings"
)

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
