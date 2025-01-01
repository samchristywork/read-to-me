package main

import (
	"database/sql"
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

var dbMutex sync.Mutex

//go:embed template/*
var templateFiles embed.FS

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
		log.Printf("%s: %v", "Error creating posts table", err)
		return
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS audio (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		hash TEXT NOT NULL UNIQUE,
		text TEXT NOT NULL,
		audio BLOB,
		audio_length_ms INTEGER
	)`)
	if err != nil {
		log.Printf("%s: %v", "Error creating audio table", err)
		return
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS collections (
		id INTEGER PRIMARY KEY,
		title TEXT,
		description TEXT,
		author TEXT,
		timestamp TEXT
	)`)
	if err != nil {
		log.Printf("%s: %v", "Error creating collections table", err)
		return
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS collection_posts (
		collection_id INTEGER,
		post_id INTEGER
	)`)
	if err != nil {
		log.Printf("%s: %v", "Error creating collection_posts table", err)
		return
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Request: %s", r.URL.Path)

		switch r.URL.Path {
		case "/":
			viewPostsHandler(w, r)
		case "/post":
			viewPostHandler(w, r)
		case "/collection":
			viewCollectionHandler(w, r)
		case "/collections":
			viewCollectionsHandler(w, r)
		case "/audio":
			audioHandler(w, r)
		case "/edit-post":
			editPostHandler(w, r)
		case "/create-post":
			createPostHandler(w, r)
		case "/create-collection":
			createCollectionHandler(w, r)
		case "/new-post":
			newPostHandler(w, r)
		case "/new-collection":
			newCollectionHandler(w, r)
		default:
			fs := http.FileServer(http.Dir("./static"))
			filePath := "./static" + r.URL.Path
			_, err := os.Stat(filePath)
			if os.IsNotExist(err) {
				httpError(w, "Page not found", http.StatusNotFound)
			} else {
				fs.ServeHTTP(w, r)
			}
		}
	})

	fmt.Println("Starting server on :4343")
	err = http.ListenAndServe(":4343", mux)
	if err != nil {
		log.Printf("%s: %v", "Error starting server", err)
	}
}
