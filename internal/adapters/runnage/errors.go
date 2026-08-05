package runnage

import (
	"fmt"
	"strings"
)

type Error struct {
	Code       string
	Message    string
	Definitive bool
}

func (err *Error) Error() string {
	if err == nil {
		return "Runnage provider error"
	}
	code := strings.TrimSpace(err.Code)
	message := strings.TrimSpace(err.Message)
	if code == "" {
		return "Runnage provider error: " + message
	}
	return fmt.Sprintf("Runnage provider error: code %q message %q", code, message)
}

func (err *Error) SafeToFallback() bool {
	return err != nil && err.Definitive
}
