package main

import (
	"context"
	"database/sql"
	"log"
	"time"

	dbpkg "github.com/James-Wolfley/steam-achievement-tracker/db"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	// 1) Open DB + apply migrations
	sqlDB, err := dbpkg.Open("data/app.db")
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer func(db *sql.DB) { _ = db.Close() }(sqlDB)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := dbpkg.ApplyMigrations(ctx, sqlDB, "db/migrations"); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	// 2) Repo + app container
	repo := dbpkg.NewRepo(sqlDB)
	app := &Application{DB: sqlDB, Repo: repo}

	// 3) Echo
	server := echo.New()
	server.Use(middleware.Logger())
	server.Use(middleware.Recover())

	server.Static("/css", "css")
	server.Static("/images", "images")
	server.Static("/scripts", "scripts")

	server.GET("/", app.Home)
	server.GET("/ui/results", app.UIResults)
	server.POST("/ui/refresh", app.UIRefresh)

	server.GET("/api/results/:steamid", app.APIResults)
	server.GET("/export/:steamid.csv", app.ExportCSV)
	server.POST("/api/refresh/:steamid", app.Refresh)

	server.Logger.Fatal(server.Start(":8080"))
}
