package verify

import "testing"

func TestValidateCheckAcceptsSupportedChallengeLengths(t *testing.T) {
	t.Parallel()

	for _, code := range []string{"1234", "123456", "1234567890"} {
		code := code
		t.Run(code, func(t *testing.T) {
			t.Parallel()
			validated, err := validateCheck(CheckRequest{Code: "  " + code + "  "})
			if err != nil {
				t.Fatalf("validateCheck() error = %v", err)
			}
			if validated.Code != code {
				t.Fatalf("validateCheck().Code = %q, want %q", validated.Code, code)
			}
		})
	}
}

func TestValidateCheckRejectsUnsupportedFormats(t *testing.T) {
	t.Parallel()

	for _, code := range []string{"123", "12345678901", "12a4", ""} {
		code := code
		t.Run(code, func(t *testing.T) {
			t.Parallel()
			if _, err := validateCheck(CheckRequest{Code: code}); err == nil {
				t.Fatalf("validateCheck() accepted %q", code)
			}
		})
	}
}
