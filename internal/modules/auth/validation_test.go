package auth

import "testing"

func TestValidateCredentials(t *testing.T) {
	email, name, password, err := validateCredentials(" Person@Example.COM ", " Person ", " 123456789012 ")
	if err != nil {
		t.Fatal(err)
	}
	if email != "person@example.com" || name != "Person" || password != "123456789012" {
		t.Fatalf("validateCredentials() = %q, %q, %q", email, name, password)
	}
	for _, test := range []RegisterRequest{
		{Email: "invalid", Name: "Person", Password: "123456789012"},
		{Email: "person@example.com", Name: " ", Password: "123456789012"},
		{Email: "person@example.com", Name: "Person", Password: "short"},
	} {
		if _, _, _, err := validateCredentials(test.Email, test.Name, test.Password); err == nil {
			t.Fatalf("validateCredentials(%#v) accepted invalid input", test)
		}
	}
}

func TestValidateResetPasswordRequest(t *testing.T) {
	request := ResetPasswordRequest{Email: "Person@Example.com", Token: " token ", Password: " 123456789012 "}
	email, token, password, err := validateResetPasswordRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if email != "person@example.com" || token != "token" || password != "123456789012" {
		t.Fatalf("validateResetPasswordRequest() = %q, %q, %q", email, token, password)
	}
}

func TestVerificationIdentifiersNormalizeEmail(t *testing.T) {
	if got := emailVerificationIdentifier(" Person@Example.COM "); got != "email.verify:person@example.com" {
		t.Fatalf("emailVerificationIdentifier() = %q", got)
	}
	if got := passwordResetIdentifier(" Person@Example.COM "); got != "password.reset:person@example.com" {
		t.Fatalf("passwordResetIdentifier() = %q", got)
	}
}
