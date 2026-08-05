package senderid

import "testing"

func TestValidateName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "letters", value: "Dugble"},
		{name: "alphanumeric", value: "DUGBLE1"},
		{name: "trimmed", value: "  Dugble1  "},
		{name: "empty", value: "", wantErr: true},
		{name: "numeric", value: "12345", wantErr: true},
		{name: "space", value: "Dugble Pay", wantErr: true},
		{name: "hyphen", value: "Dugble-1", wantErr: true},
		{name: "underscore", value: "Dugble_1", wantErr: true},
		{name: "unicode", value: "Dúgble", wantErr: true},
		{name: "too long", value: "DugbleSender", wantErr: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateName(test.value)
			if test.wantErr && err == nil {
				t.Fatalf("ValidateName(%q) error = nil", test.value)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("ValidateName(%q) error = %v", test.value, err)
			}
		})
	}
}
