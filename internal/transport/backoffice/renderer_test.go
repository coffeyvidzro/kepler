package backoffice

import "testing"

func TestNewRendererParsesTemplates(t *testing.T) {
	t.Parallel()

	renderer, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer() error = %v", err)
	}
	if renderer == nil || renderer.templates == nil {
		t.Fatal("NewRenderer() returned an unconfigured renderer")
	}
}
