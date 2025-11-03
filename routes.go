package main

import (
	"net/http"

	"github.com/James-Wolfley/steam-achievement-tracker/views"
	"github.com/labstack/echo/v4"
)

func (app *Application) Index(c echo.Context) error {
	return render(c, http.StatusOK, views.Index(views.Home))
}

func (app *Application) Home(c echo.Context) error {
	if c.Request().Header.Get("HX-Request") != "" {
		return render(c, http.StatusOK, views.Home())
	} else {
		return render(c, http.StatusOK, views.Index(views.Home))
	}
}
