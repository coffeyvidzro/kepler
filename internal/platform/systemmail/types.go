package systemmail

import platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"

type Recipient struct {
	Name  string
	Email string
}

type SendEmailVerificationInput struct {
	ToEmail string
	Name    string
	Token   string
}

type SendPasswordResetInput struct {
	ToEmail string
	Name    string
	Token   string
}

type SendPasswordChangedInput struct {
	ToEmail string
	Name    string
}

type SendEmailChangedInput struct {
	ToEmail string
	Name    string
	Email   string
}

type SendSecurityEventInput struct {
	ToEmail string
	Name    string
}

type SendNewLoginInput struct {
	ToEmail   string
	Name      string
	IPAddress string
	UserAgent string
	Method    string
}

type SendTeamMemberChangedInput struct {
	ToEmail string
	Name    string
	Team    string
	Role    string
}

type SendTeamTokenChangedInput struct {
	ToEmail     string
	Name        string
	TeamID      string
	TokenName   string
	TokenPrefix string
}

type SendTeamInvitationInput struct {
	ToEmail     string
	Name        string
	TeamName    string
	InviterName string
	Role        string
	Token       string
}

type EmailSender = platformemail.Sender
