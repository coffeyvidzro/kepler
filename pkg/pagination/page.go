package pagination

import (
	"errors"
	"fmt"
)

const (
	DefaultLimit = 25
	MaxLimit     = 100
)

var (
	ErrInvalidLimit  = errors.New("pagination limit must be positive")
	ErrLimitTooLarge = errors.New("pagination limit exceeds the maximum")
	ErrInvalidOffset = errors.New("pagination offset must not be negative")
)

// PageRequest describes offset pagination input.
type PageRequest struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// NewPageRequest validates pagination input. A zero limit selects DefaultLimit.
func NewPageRequest(limit, offset int) (PageRequest, error) {
	if limit == 0 {
		limit = DefaultLimit
	}
	request := PageRequest{Limit: limit, Offset: offset}
	if err := request.Validate(); err != nil {
		return PageRequest{}, err
	}
	return request, nil
}

// NormalizePageRequest applies defaults and clamps unsafe values for callers
// that prefer normalization over strict validation.
func NormalizePageRequest(limit, offset int) PageRequest {
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	if offset < 0 {
		offset = 0
	}
	return PageRequest{Limit: limit, Offset: offset}
}

func (request PageRequest) Validate() error {
	if request.Limit <= 0 {
		return ErrInvalidLimit
	}
	if request.Limit > MaxLimit {
		return fmt.Errorf("%w: maximum is %d", ErrLimitTooLarge, MaxLimit)
	}
	if request.Offset < 0 {
		return ErrInvalidOffset
	}
	return nil
}

func (request PageRequest) NextOffset() int {
	return request.Offset + request.Limit
}

func (request PageRequest) PageNumber() int {
	if request.Limit <= 0 {
		return 1
	}
	return request.Offset/request.Limit + 1
}

// Page is an offset-paginated response.
type Page[T any] struct {
	Items      []T   `json:"items"`
	Limit      int   `json:"limit"`
	Offset     int   `json:"offset"`
	Total      int64 `json:"total"`
	HasMore    bool  `json:"has_more"`
	NextOffset *int  `json:"next_offset,omitempty"`
}

func NewPage[T any](items []T, request PageRequest, total int64) Page[T] {
	if total < 0 {
		total = 0
	}
	hasMore := int64(request.Offset+len(items)) < total
	var nextOffset *int
	if hasMore {
		value := request.NextOffset()
		nextOffset = &value
	}
	return Page[T]{
		Items:      items,
		Limit:      request.Limit,
		Offset:     request.Offset,
		Total:      total,
		HasMore:    hasMore,
		NextOffset: nextOffset,
	}
}
