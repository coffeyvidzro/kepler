package backoffice

import (
	"context"
	"errors"
	"net/http"

	httptransport "github.com/coffeyvidzro/dugble/server/internal/transport/http"
)

// Application owns the backoffice HTTP transport server.
type Application struct {
	server *httptransport.Server
}

func NewApplication(handler http.Handler, address string) (*Application, error) {
	server, err := httptransport.NewServer(handler, address)
	if err != nil {
		return nil, err
	}
	return &Application{server: server}, nil
}

func (application *Application) Run(ctx context.Context) error {
	if application == nil || application.server == nil {
		return errors.New("backoffice HTTP application is not configured")
	}
	return application.server.Run(ctx)
}
