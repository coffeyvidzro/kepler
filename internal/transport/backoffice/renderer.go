package backoffice

import (
	"fmt"
	"html/template"
	"io"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/backofficeweb"
)

type Renderer struct {
	templates *template.Template
}

func NewRenderer() (*Renderer, error) {
	tmpl, err := template.New("").
		Funcs(template.FuncMap{
			"money":         formatMoney,
			"microsInput":   formatMicrosInput,
			"datetimeLocal": formatDatetimeLocal,
		}).
		ParseFS(backofficeweb.Files, "templates/*.html")
	if err != nil {
		return nil, err
	}

	return &Renderer{templates: tmpl}, nil
}

func (r *Renderer) Render(_ *echo.Context, w io.Writer, templateName string, data any) error {
	return r.templates.ExecuteTemplate(w, templateName, data)
}

func formatMoney(micros int64) string {
	negative := micros < 0
	if negative {
		micros = -micros
	}

	whole := micros / 1_000_000
	fraction := micros % 1_000_000
	fractionText := fmt.Sprintf("%06d", fraction)
	for len(fractionText) > 2 && fractionText[len(fractionText)-1] == '0' {
		fractionText = fractionText[:len(fractionText)-1]
	}

	if negative {
		return fmt.Sprintf("-$%d.%s", whole, fractionText)
	}
	return fmt.Sprintf("$%d.%s", whole, fractionText)
}

func formatMicrosInput(micros int64) string {
	return fmt.Sprintf("%d.%06d", micros/1_000_000, micros%1_000_000)
}

func formatDatetimeLocal(value any) string {
	var timestamp time.Time
	switch typed := value.(type) {
	case time.Time:
		timestamp = typed
	case *time.Time:
		if typed == nil {
			return ""
		}
		timestamp = *typed
	default:
		return ""
	}
	return timestamp.UTC().Format("2006-01-02T15:04")
}
