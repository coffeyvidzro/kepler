package domains

import (
	"context"
	"errors"
	"testing"
)

func TestUpdateStatusRejectsInvalidRequests(t *testing.T) {
	t.Parallel()

	svc := NewService(nil)

	for _, req := range []StatusRequest{
		{Action: "hold", Reason: "bad action"},
		{Action: "fail"},
	} {
		req := req
		t.Run(req.Action+req.Reason, func(t *testing.T) {
			t.Parallel()

			err := svc.UpdateStatus(context.Background(), "domain-id", req)
			if !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("UpdateStatus() error = %v, want ErrInvalidRequest", err)
			}
		})
	}
}
