package backoffice

import (
	"errors"
	"fmt"

	"github.com/labstack/echo/v5"
)

func RegisterWeb(router *echo.Echo) error {
	if router == nil {
		return errors.New("backoffice router is required")
	}

	renderer, err := NewRenderer()
	if err != nil {
		return fmt.Errorf("create backoffice renderer: %w", err)
	}
	router.Renderer = renderer

	if err := RegisterAssets(router); err != nil {
		return fmt.Errorf("register backoffice assets: %w", err)
	}

	return nil
}
