package user

import "time"

// User represents a user in the system.
type User struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	EmailVerified bool      `json:"email_verified"`
	Name          string    `json:"name"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type UpdateProfileRequest struct {
	Name string `json:"name"`
}

type UpdateEmailRequest struct {
	Email string `json:"email"`
}

type UpdatePasswordRequest struct {
	Password string `json:"password"`
}
