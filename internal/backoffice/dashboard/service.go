package dashboard

import "context"

type Service struct {
	repository *Repository
}

func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Stats(ctx context.Context) (Stats, error) {
	return s.repository.Stats(ctx)
}
