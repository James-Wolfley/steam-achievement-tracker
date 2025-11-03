package main

import (
	"database/sql"

	"github.com/James-Wolfley/steam-achievement-tracker/db"
)

type Application struct {
	DB   *sql.DB
	Repo db.Repo
}
