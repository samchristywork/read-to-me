package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
)

func editPostHandler(w http.ResponseWriter, r *http.Request, errFunc httpErrorFunc) {
	id := r.URL.Query().Get("id")
	if id == "" {
		errFunc(w, "Post ID is required", http.StatusBadRequest)
		return
	}

	stmt, err := db.Prepare(`SELECT title, content, author, source, timestamp
		FROM posts WHERE id = ?`)
	if err != nil {
		errFunc(w, "Unable to retrieve post", http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	var title, body, author, source, timestamp string
	err = stmt.QueryRow(id).Scan(&title, &body, &author, &source, &timestamp)
	if err != nil {
		if err == sql.ErrNoRows {
			errFunc(w, "Post not found", http.StatusNotFound)
			return
		}
		errFunc(w, "Unable to retrieve post", http.StatusInternalServerError)
		return
	}

	content := createPost(title, source, body)
	page, err := renderPage(content)

	if err != nil {
		errFunc(w, "Unable to load page", http.StatusInternalServerError)
		return
	}

	_, err = fmt.Fprintln(w, page)
	if err != nil {
		log.Printf("%s: %v", "Error writing response", err)
	}
}
