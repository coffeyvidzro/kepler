package backoffice

import (
	"context"
	"testing"
)

type pingDatabase struct {
	called bool
}

func (database *pingDatabase) Ping(context.Context) error {
	database.called = true
	return nil
}

func TestNewServiceRejectsMissingDatabase(t *testing.T) {
	t.Parallel()

	if _, err := NewService(nil); err == nil {
		t.Fatal("expected an error for a missing database")
	}
}

func TestServiceReadyPingsDatabase(t *testing.T) {
	t.Parallel()

	database := &pingDatabase{}
	service, err := NewService(database)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	if err := service.Ready(context.Background()); err != nil {
		t.Fatalf("check readiness: %v", err)
	}
	if !database.called {
		t.Fatal("expected readiness to ping the database")
	}
}
