package dispatch

import (
	"fmt"
	"time"

	"github.com/coffeyvidzro/dugble/server/internal/platform/systemmail"
)

const (
	verificationEmailFromName    = "Argus"
	verificationEmailSubject     = "Your verification code"
	verificationEmailPreviewText = "Use this code to complete verification."
)

type verificationEmailMessage struct {
	FromName string
	Subject  string
	Text     string
	HTML     string
}

func buildVerificationEmail(renderer *systemmail.ArgusRenderer, code string, remaining time.Duration) (verificationEmailMessage, error) {
	expiresIn := formatVerificationExpiry(remaining)
	htmlBody, err := renderer.RenderVerification(systemmail.ArgusVerificationTemplateData{
		PreviewText: verificationEmailPreviewText,
		Code:        code,
		ExpiresIn:   expiresIn,
	})
	if err != nil {
		return verificationEmailMessage{}, err
	}
	return verificationEmailMessage{
		FromName: verificationEmailFromName,
		Subject:  verificationEmailSubject,
		Text: fmt.Sprintf(
			"Your verification code is %s.\n\nThis code expires in %s.\n\nDo not share this code with anyone.\n\nIf you did not request this code, you can safely ignore this email.",
			code,
			expiresIn,
		),
		HTML: htmlBody,
	}, nil
}

func buildVerificationSMS(code string, remaining time.Duration) string {
	return fmt.Sprintf(
		"%s is your Dugble verification code. It expires in %s. Do not share it.",
		code,
		formatVerificationExpiry(remaining),
	)
}

func formatVerificationExpiry(remaining time.Duration) string {
	minutes := int((remaining + time.Minute - 1) / time.Minute)
	if minutes < 1 {
		minutes = 1
	}
	if minutes == 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", minutes)
}
