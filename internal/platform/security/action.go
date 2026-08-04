package security

// Action identifies a semantic operation evaluated by the security runtime.
type Action string

const (
	ActionHTTPRequest  Action = "http.request"
	ActionAuthLogin    Action = "auth.login"
	ActionAuthRegister Action = "auth.register"
	ActionEmailSend    Action = "email.send"
)
