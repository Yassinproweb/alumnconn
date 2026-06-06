package db

import (
	"database/sql"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

func ConnectDB() {
	var err error

	DB, err = sql.Open("sqlite3", "./users.db")
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

	log.Println("SQLite Connected")
}

func createTables() {
	query := `
		CREATE TABLE IF NOT EXISTS users(
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL,
			email TEXT UNIQUE NOT NULL,
			password TEXT NOT NULL,
			role TEXT NOT NULL,
			faculty TEXT NOT NULL,
			entry_year TEXT NOT NULL,
			bio TEXT,
			avatar TEXT,
			avatar_color TEXT DEFAULT '#006633'
		);

	  CREATE TABLE IF NOT EXISTS posts(
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      user_id INTEGER,
      content TEXT,
      media_name TEXT,
      media_type TEXT,
      image TEXT,
      created_at DATETIME DEFAULT CURRENT_TIMESTAMP,

      FOREIGN KEY(user_id)
      REFERENCES users(id)
    );

    CREATE TABLE IF NOT EXISTS likes(
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      user_id INTEGER NOT NULL,
      post_id INTEGER NOT NULL,
      created_at DATETIME DEFAULT CURRENT_TIMESTAMP,

      UNIQUE(user_id, post_id),

      FOREIGN KEY(user_id) REFERENCES users(id),
      FOREIGN KEY(post_id) REFERENCES posts(id)
    );

		CREATE TABLE IF NOT EXISTS messages (
	    id         INTEGER PRIMARY KEY AUTOINCREMENT,
	    sender_id  INTEGER NOT NULL,
	    receiver_id INTEGER NOT NULL,
	    content    TEXT NOT NULL DEFAULT '',
	    media_name TEXT,
	    media_type TEXT,
	    is_read    INTEGER NOT NULL DEFAULT 0,
	    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS comments (
 	    id INTEGER PRIMARY KEY AUTOINCREMENT,
 	    post_id INTEGER NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
      user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
      parent_id INTEGER REFERENCES comments(id) ON DELETE CASCADE, -- NULL = top-level
  	  content TEXT NOT NULL,
   	  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS comment_votes (
   	  id INTEGER PRIMARY KEY AUTOINCREMENT,
    	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 	    comment_id INTEGER NOT NULL REFERENCES comments(id) ON DELETE CASCADE,
  	  vote INTEGER NOT NULL CHECK(vote IN (1, -1)), -- 1=thumbs up, -1=thumbs down
   		UNIQUE(user_id, comment_id)
		);
	`

	_, err := DB.Exec(query)
	if err != nil {
		log.Fatal(err)
	}
}
