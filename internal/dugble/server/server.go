package server

import (
	"context"
	"errors"
	"net/http"

	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
	platformwebhook "github.com/coffeyvidzro/dugble/server/internal/platform/webhook"
	httptransport "github.com/coffeyvidzro/dugble/server/internal/transport/http"
)

// Runtime contains application-level services shared by server handlers.
type Runtime struct{ Events *platformevent.Emitter }

func New(dependencies Dependencies)(*Runtime,error){if err:=dependencies.validate();err!=nil{return nil,err};return &Runtime{Events:platformevent.NewEmitter(platformwebhook.NewEventSink(dependencies.WebhookEmitter))},nil}

// Application owns the refactored HTTP transport server.
type Application struct{ server *httptransport.Server }

func NewApplication(handler http.Handler,address string)(*Application,error){server,err:=httptransport.NewServer(handler,address);if err!=nil{return nil,err};return &Application{server:server},nil}

func(application *Application)Run(ctx context.Context)error{if application==nil||application.server==nil{return errors.New("HTTP application is not configured")};return application.server.Run(ctx)}
