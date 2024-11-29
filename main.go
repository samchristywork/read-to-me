package main

import (
	"net/http"
	"fmt"
	"log"
	"database/sql"
	"os"
	"embed"
	"strings"
	"html/template"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed template/*
var templateFiles embed.FS

var db *sql.DB

func post(id, title, url, content, author, timestamp, class string) template.HTML {
	return template.HTML(fmt.Sprintf(`
		<div class="post %s">
			<h3><a href="/post?id=%s">%s</a> - <a href="%s">%s</a></h3>
			<em data-timestamp="%s">%s at %s</em>
			<div class="post-body">%s</div>
		</div>
	`, class, id, title, url, "source", timestamp, author, timestamp, content))
}

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

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	page, err := renderPage("<main><h1>Error - Page not found</h1></main>")
	if err != nil {
		http.Error(w, "Internal Server Error: Unable to load page", http.StatusInternalServerError)
		return
	}

	if _, err := fmt.Fprintln(w, page); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, title, source, content, author, timestamp FROM posts")
	if err != nil {
		log.Printf("Error querying posts: %v", err)
		http.Error(w, "Internal Server Error: Unable to load posts", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var contentBuilder strings.Builder
	contentBuilder.WriteString(`<main>`)

	for rows.Next() {
		var id int
		var title, source, body, author, timestamp string
		if err := rows.Scan(&id, &title, &source, &body, &author, &timestamp); err != nil {
			log.Printf("Error scanning row: %v", err)
			http.Error(w, "Internal Server Error: Unable to read post data", http.StatusInternalServerError)
			return
		}

		uniqueID := fmt.Sprintf("%d", id)
		body = template.HTMLEscapeString(body)

		contentBuilder.WriteString(string(post(uniqueID, title, source, body, author, timestamp, "short")))
	}

	if err := rows.Err(); err != nil {
		log.Printf("Error after iterating rows: %v", err)
		http.Error(w, "Internal Server Error: Unable to load posts", http.StatusInternalServerError)
		return
	}

	page, err := renderPage(contentBuilder.String())
	if err != nil {
		http.Error(w, "Internal Server Error: Unable to load page", http.StatusInternalServerError)
		return
	}

	if _, err := fmt.Fprintln(w, page); err != nil {
		log.Printf("Error writing response: %v", err)
	}
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
		case "/":
			homeHandler(w, r)
		default:
			fs := http.FileServer(http.Dir("./static"))
			filePath := "./static" + r.URL.Path
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				notFoundHandler(w, r)
			} else {
				fs.ServeHTTP(w, r)
			}
		}
	})

	fmt.Println("Starting server on :4343")
	if err = http.ListenAndServe(":4343", mux); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}
