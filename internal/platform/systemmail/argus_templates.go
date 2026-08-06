package systemmail

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"html/template"
)

const argusVerificationTemplate = "verification.html"

//go:embed templates/argus/*.html
var argusTemplateFS embed.FS

type ArgusVerificationTemplateData struct {
	PreviewText string
	Code        string
	ExpiresIn   string
}

type ArgusRenderer struct {
	tmpl *template.Template
}

func NewArgusRenderer() (*ArgusRenderer, error) {
	tmpl, err := template.ParseFS(argusTemplateFS, "templates/argus/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse Argus email templates: %w", err)
	}
	return &ArgusRenderer{tmpl: tmpl}, nil
}

func (renderer *ArgusRenderer) RenderVerification(data ArgusVerificationTemplateData) (string, error) {
	if renderer == nil || renderer.tmpl == nil {
		return "", errors.New("Argus email renderer is not configured")
	}
	var body bytes.Buffer
	if err := renderer.tmpl.ExecuteTemplate(&body, argusVerificationTemplate, data); err != nil {
		return "", fmt.Errorf("render Argus verification template: %w", err)
	}
	return body.String(), nil
}
