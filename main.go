package main

import (
	"net/http"
	"fmt"
	"log"
	"database/sql"
	"os"
	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

func main() {
	var err error
	db, err = sql.Open("sqlite3", "./data.db")
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS posts (
		id INTEGER PRIMARY KEY,
		title TEXT,
		source TEXT,
		content TEXT,
		author TEXT,
		timestamp TEXT
	)`)
	if err != nil {
		log.Fatalf("Error creating posts table: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS audio (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		hash TEXT NOT NULL UNIQUE,
		text TEXT NOT NULL,
		audio BLOB,
		audio_length_ms INTEGER
	)`)
	if err != nil {
		log.Fatalf("Error creating audio table: %v", err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		default:
			fs := http.FileServer(http.Dir("./static"))
			fs.ServeHTTP(w, r)
		}
	})

	fmt.Println("Starting server on :4343")
	if err = http.ListenAndServe(":4343", mux); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}
