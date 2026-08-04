package server

import (
	"testing"

	platformwebhook "github.com/coffeyvidzro/dugble/server/internal/platform/webhook"
)

func TestNewRequiresWebhookEmitter(t *testing.T) {
	if _, err := New(Dependencies{}); err == nil {
		t.Fatal("New() accepted missing dependencies")
	}
}

func TestNewBuildsEventRuntime(t *testing.T) {
	runtime, err := New(Dependencies{WebhookEmitter: platformwebhook.NewEmitter(nil)})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if runtime.Events == nil {
		t.Fatal("New() did not build an event emitter")
	}
}
