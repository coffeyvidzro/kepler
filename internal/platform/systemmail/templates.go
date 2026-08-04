package systemmail

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
)

//go:embed templates/*.html
var templateFS embed.FS

type Renderer struct {
	tmpl *template.Template
}

func NewRenderer() (*Renderer, error) {
	// Parse all files at once
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Renderer{tmpl: tmpl}, nil
}

// Render now lives on the struct
func (r *Renderer) Render(templateName string, data any) (string, error) {
	var buf bytes.Buffer
	err := r.tmpl.ExecuteTemplate(&buf, templateName, data)
	if err != nil {
		return "", fmt.Errorf("failed to render template %s: %w", templateName, err)
	}
	return buf.String(), nil
}
