package senderids

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidRequest = errors.New("invalid sender id request")

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
	case "approve":
		return s.repository.Approve(ctx, strings.TrimSpace(id))
	case "reject":
		reason := strings.TrimSpace(req.Reason)
		if reason == "" {
			return fmt.Errorf("%w: rejection reason is required", ErrInvalidRequest)
		}
		return s.repository.Reject(ctx, strings.TrimSpace(id), reason)
	default:
		return fmt.Errorf("%w: action must be approve or reject", ErrInvalidRequest)
	}
}
