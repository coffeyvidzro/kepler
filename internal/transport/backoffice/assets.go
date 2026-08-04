package backoffice

import (
	"io/fs"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/backofficeweb"
)

func RegisterAssets(router *echo.Echo) error {
	staticFS, err := fs.Sub(backofficeweb.Files, "static")
	if err != nil {
		return err
	}

	router.GET("/static/*", echo.WrapHandler(http.StripPrefix(
		"/static/",
		http.FileServer(http.FS(staticFS)),
	)))

	return nil
}
