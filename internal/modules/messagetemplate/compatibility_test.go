package messagetemplate

import (
	"encoding/json"
	"testing"
)

func TestStringListAcceptsStringAndArray(t *testing.T) {
	for _, test := range []struct {
		input string
		want  int
	}{{`"reply@example.com"`, 1}, {`["a@example.com","b@example.com"]`, 2}} {
		var values StringList
		if err := json.Unmarshal([]byte(test.input), &values); err != nil {
			t.Fatalf("unmarshal %s: %v", test.input, err)
		}
		if len(values) != test.want {
			t.Fatalf("unmarshal %s length = %d, want %d", test.input, len(values), test.want)
		}
	}
}

func TestTemplateMutationResponseContract(t *testing.T) {
	data, err := json.Marshal(MutationResponse{Object: ObjectTemplate, ID: "template-id"})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"object":"template","id":"template-id"}` {
		t.Fatalf("unexpected response: %s", data)
	}
}

func TestNormalizeAPIListRequest(t *testing.T) {
	request := APIListRequest{}
	if err := normalizeAPIListRequest(&request); err != nil {
		t.Fatal(err)
	}
	if request.Limit != 20 {
		t.Fatalf("default limit = %d", request.Limit)
	}
	if err := normalizeAPIListRequest(&APIListRequest{Limit: 101}); err == nil {
		t.Fatal("expected invalid limit")
	}
	if err := normalizeAPIListRequest(&APIListRequest{After: uuidText, Before: uuidText}); err == nil {
		t.Fatal("expected mutually exclusive cursors")
	}
}

const uuidText = "b6d24b8e-af0b-4c3c-be0c-359bbd97381e"
