package auth

import authmodule "github.com/coffeyvidzro/dugble/server/internal/modules/auth"

type Service = authmodule.Service
type RegisterRequest = authmodule.RegisterRequest
type LoginRequest = authmodule.LoginRequest
type MFALoginRequest = authmodule.MFALoginRequest
type VerifyEmailRequest = authmodule.VerifyEmailRequest
type ResendEmailRequest = authmodule.ResendEmailRequest
type ForgotPasswordRequest = authmodule.ForgotPasswordRequest
type ResetPasswordRequest = authmodule.ResetPasswordRequest
