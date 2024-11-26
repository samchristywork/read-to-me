package main

import (
	"net/http"
	"fmt"
	"log"
	"database/sql"
	"os"
	"embed"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed template/*
var templateFiles embed.FS

var db *sql.DB

func renderPage(content string) (string, error) {
	tmpl, err := templateFiles.ReadFile("template/head.html")
	if err != nil {
		log.Printf("Error reading head template: %v", err)
		return "", err
	}
	head := string(tmpl)

	tmpl, err = templateFiles.ReadFile("template/nav.html")
	if err != nil {
		log.Printf("Error reading nav template: %v", err)
		return "", err
	}
	nav := string(tmpl)

	tmpl, err = templateFiles.ReadFile("template/footer.html")
	if err != nil {
		log.Printf("Error reading footer template: %v", err)
		return "", err
	}
	footer := string(tmpl)

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>%s</head>
<body>%s%s%s</body>
</html>`, head, nav, content, footer), nil
}

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
