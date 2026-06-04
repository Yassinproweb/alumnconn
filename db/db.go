package db

import (
	"database/sql"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

func ConnectDB() {
	var err error

	DB, err = sql.Open("sqlite3", "./pos.db")
	if err != nil {
		log.Fatal(err)
	}

	_, err = DB.Exec(`
		PRAGMA foreign_keys = ON;
		PRAGMA journal_mode = WAL;
	`)

	if err != nil {
		log.Fatal(err)
	}

	createTables()
	SeedDB()

	log.Println("SQLite Connected")
}

func createTables() {
	query := `
		CREATE TABLE IF NOT EXISTS users(
			id INT PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT UNIQUE NOT NULL,
			name TEXT NOT NULL,
			password TEXT NOT NULL,
			role TEXT NOT NULL,
			faculty TEXT NOT NULL,
			grad_year TEXT NOT NULL,
			bio TEXT NOT NULL,
		)
	`
}
