package contact

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestContactTopicListResponseContract(t *testing.T) {
	description := "New features and announcements"
	response := ContactTopicListResponse{
		Object:  ObjectList,
		HasMore: false,
		Data: []ContactTopic{{
			ID:           "b6d24b8e-af0b-4c3c-be0c-359bbd97381e",
			Name:         "Product Updates",
			Description:  &description,
			Subscription: SubscriptionOptIn,
		}},
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal contact topic list response: %v", err)
	}
	body := string(encoded)
	for _, required := range []string{"object", "list", "has_more", "data", "subscription", "opt_in"} {
		if !strings.Contains(body, required) {
			t.Fatalf("response omitted %q: %s", required, body)
		}
	}
	for _, forbidden := range []string{"team_id", "default_subscription", "visibility", "created_at", "updated_at"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked internal field %q: %s", forbidden, body)
		}
	}
}

func TestNormalizeContactTopicListRequest(t *testing.T) {
	request := ListContactTopicsRequest{}
	if err := normalizeContactTopicListRequest(&request); err != nil {
		t.Fatalf("normalize default request: %v", err)
	}
	if request.Limit != 20 {
		t.Fatalf("expected default limit 20, got %d", request.Limit)
	}

	request = ListContactTopicsRequest{Limit: 10, After: "a", Before: "b"}
	if err := normalizeContactTopicListRequest(&request); err == nil {
		t.Fatal("expected simultaneous cursors to fail")
	}

	request = ListContactTopicsRequest{Limit: 101}
	if err := normalizeContactTopicListRequest(&request); err == nil {
		t.Fatal("expected out-of-range limit to fail")
	}
}

func TestValidateContactTopicUpdates(t *testing.T) {
	updates, err := validateContactTopicUpdates(UpdateContactTopicsRequest{{
		ID:           " b6d24b8e-af0b-4c3c-be0c-359bbd97381e ",
		Subscription: " OPT_OUT ",
	}})
	if err != nil {
		t.Fatalf("validate topic updates: %v", err)
	}
	if updates[0].ID != "b6d24b8e-af0b-4c3c-be0c-359bbd97381e" || updates[0].Subscription != SubscriptionOptOut {
		t.Fatalf("unexpected normalized update: %#v", updates[0])
	}

	for name, request := range map[string]UpdateContactTopicsRequest{
		"empty":                {},
		"invalid id":           {{ID: "not-a-uuid", Subscription: SubscriptionOptIn}},
		"invalid subscription": {{ID: "b6d24b8e-af0b-4c3c-be0c-359bbd97381e", Subscription: "subscribed"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := validateContactTopicUpdates(request); err == nil {
				t.Fatal("expected validation failure")
			}
		})
	}
}

func TestValidateContactIdentifier(t *testing.T) {
	for _, value := range []string{
		"e169aa45-1ecf-4183-9955-b1499d5701d3",
		"steve.wozniak@gmail.com",
	} {
		if _, err := validateContactIdentifier(value); err != nil {
			t.Fatalf("validate %q: %v", value, err)
		}
	}
	if _, err := validateContactIdentifier("not a contact"); err == nil {
		t.Fatal("expected invalid contact identifier to fail")
	}
}
