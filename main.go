package main

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)




func main() {
  // Echo instance
  server := echo.New()
  // Middleware
  server.Use(middleware.Logger())
  server.Use(middleware.Recover())

  app := &Application{}

	server.Static("/css", "css")
	server.Static("/images", "images")
	server.Static("/scripts", "scripts")

  server.GET("/", func(c echo.Context) error {
    return c.Redirect(http.StatusMovedPermanently, "/home")
  })

	server.GET("/home", app.Home)
  // Start server
  server.Logger.Fatal(server.Start(":8080"))
}