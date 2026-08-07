package topic

import (
	"encoding/json"
	"testing"
)

func TestTopicResponseContracts(t *testing.T) {
	mutation, err := json.Marshal(MutationResponse{Object: ObjectTopic, ID: "topic-id"})
	if err != nil {
		t.Fatal(err)
	}
	if string(mutation) != `{"object":"topic","id":"topic-id"}` {
		t.Fatalf("unexpected mutation response: %s", mutation)
	}
	deleted, err := json.Marshal(DeleteResponse{Object: ObjectTopic, ID: "topic-id", Deleted: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(deleted) != `{"object":"topic","id":"topic-id","deleted":true}` {
		t.Fatalf("unexpected delete response: %s", deleted)
	}
}

func TestNormalizeTopicAPIListRequest(t *testing.T) {
	request := APIListRequest{}
	if err := normalizeAPIListRequest(&request); err != nil {
		t.Fatal(err)
	}
	if request.Limit != 20 {
		t.Fatalf("default limit = %d", request.Limit)
	}
	if err := normalizeAPIListRequest(&APIListRequest{Limit: -1}); err == nil {
		t.Fatal("expected invalid limit")
	}
	if err := normalizeAPIListRequest(&APIListRequest{After: uuidText, Before: uuidText}); err == nil {
		t.Fatal("expected mutually exclusive cursors")
	}
}

func TestCreateTopicDefaultsPrivate(t *testing.T) {
	request, err := validateCreate(CreateRequest{Name: "Newsletter", DefaultSubscription: "opt_in"})
	if err != nil {
		t.Fatal(err)
	}
	if request.Visibility != "private" {
		t.Fatalf("visibility = %q, want private", request.Visibility)
	}
}

const uuidText = "b6d24b8e-af0b-4c3c-be0c-359bbd97381e"
