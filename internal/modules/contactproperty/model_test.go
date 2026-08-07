package contactproperty

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestPropertyResourceResponseOmitsInternalFields(t *testing.T) {
	createdAt := time.Date(2026, time.April, 8, 0, 11, 13, 110779000, time.UTC)
	property := Property{
		ID:            "b6d24b8e-af0b-4c3c-be0c-359bbd97381e",
		TeamID:        "5c9ad8fa-9ae4-4a12-955f-bcab66f26fa5",
		Key:           "company_name",
		Type:          "string",
		FallbackValue: "Acme Corp",
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt.Add(time.Hour),
	}

	encoded, err := json.Marshal(property.ResourceResponse())
	if err != nil {
		t.Fatalf("marshal resource response: %v", err)
	}
	body := string(encoded)
	for _, forbidden := range []string{"team_id", "updated_at"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked internal field %q: %s", forbidden, body)
		}
	}
	for _, required := range []string{"object", "contact_property", "fallback_value", "created_at"} {
		if !strings.Contains(body, required) {
			t.Fatalf("response omitted %q: %s", required, body)
		}
	}
}

func TestPropertyMutationResponses(t *testing.T) {
	property := Property{ID: "b6d24b8e-af0b-4c3c-be0c-359bbd97381e"}
	mutation := property.MutationResponse()
	if mutation.Object != ObjectContactProperty || mutation.ID != property.ID {
		t.Fatalf("unexpected mutation response: %#v", mutation)
	}
	deleted := property.DeleteResponse()
	if deleted.Object != ObjectContactProperty || deleted.ID != property.ID || !deleted.Deleted {
		t.Fatalf("unexpected delete response: %#v", deleted)
	}
}

func TestNormalizeListRequest(t *testing.T) {
	t.Run("defaults limit", func(t *testing.T) {
		request := ListRequest{}
		if err := normalizeListRequest(&request); err != nil {
			t.Fatalf("normalize list request: %v", err)
		}
		if request.Limit != 20 {
			t.Fatalf("expected default limit 20, got %d", request.Limit)
		}
	})

	t.Run("rejects both cursors", func(t *testing.T) {
		request := ListRequest{After: "after", Before: "before"}
		if err := normalizeListRequest(&request); err == nil {
			t.Fatal("expected both cursors to be rejected")
		}
	})

	t.Run("rejects out of range limit", func(t *testing.T) {
		request := ListRequest{Limit: 101}
		if err := normalizeListRequest(&request); err == nil {
			t.Fatal("expected out of range limit to be rejected")
		}
	})
}
