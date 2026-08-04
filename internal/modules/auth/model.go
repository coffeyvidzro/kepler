package auth

import "time"

type RegisterRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type VerifyEmailRequest struct {
	Email string `json:"email"`
	Token string `json:"token"`
}

type ResendEmailRequest struct {
	Email string `json:"email"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

type ResetPasswordRequest struct {
	Email    string `json:"email"`
	Token    string `json:"token"`
	Password string `json:"password"`
}

type AuthenticatedUser struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	EmailVerified bool      `json:"email_verified"`
	Name          string    `json:"name"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type AuthResponse struct {
	User AuthenticatedUser `json:"user"`
}

type LoginResponse struct {
	User           *AuthenticatedUser `json:"user,omitempty"`
	MFARequired    bool               `json:"mfa_required"`
	ChallengeToken string             `json:"challenge_token,omitempty"`
	Methods        []string           `json:"methods,omitempty"`
}

type MFALoginRequest struct {
	ChallengeToken string `json:"challenge_token"`
	Code           string `json:"code"`
}
