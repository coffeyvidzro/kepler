package webhook

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestDocumentedEventsMatchSubscribableCatalog(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve webhook catalog test path")
	}
	documentPath := filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "..", "docs", "webhooks", "events.mdx")
	content, err := os.ReadFile(documentPath)
	if err != nil {
		t.Fatalf("read webhook event documentation: %v", err)
	}
	documented := append(documentedEvents(t, string(content), "SMS events", "sms"), documentedEvents(t, string(content), "Email events", "email")...)
	documented = append(documented, documentedEvents(t, string(content), "Verify events", "verification")...)
	if supported := SubscribableEventTypes(); !reflect.DeepEqual(documented, supported) {
		t.Fatalf("documented events = %v, subscribable events = %v", documented, supported)
	}
}

func documentedEvents(t *testing.T, document, section, resource string) []string {
	t.Helper()
	parts := strings.SplitN(document, "## "+section, 2)
	if len(parts) != 2 {
		t.Fatalf("webhook event documentation is missing the %s section", section)
	}
	body := strings.SplitN(parts[1], "\n## ", 2)[0]
	eventPattern := regexp.MustCompile(`(?m)^\| \x60(` + regexp.QuoteMeta(resource) + `\.[a-z_]+)\x60`)
	matches := eventPattern.FindAllStringSubmatch(body, -1)
	events := make([]string, 0, len(matches))
	for _, match := range matches {
		events = append(events, match[1])
	}
	return events
}

func TestSubscribableEventTypesReturnsCopy(t *testing.T) {
	events := SubscribableEventTypes()
	events[0] = "modified"
	if IsSubscribableEventType("modified") || !IsSubscribableEventType(EventSMSSubmitted) {
		t.Fatal("SubscribableEventTypes exposed mutable catalog state")
	}
}
