package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"
)

func collection(id, title, content, author, time, class string) template.HTML {
	return template.HTML(fmt.Sprintf(`
		<div class="post %s">
			<h3><a href="/collection?id=%s">%s</a></h3>
			<em data-timestamp="%s">%s at %s</em>
			<div class="post-body">%s</div>
		</div>
	`, class, id, title, time, author, time, content))
}

func viewCollectionHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		httpError(w, "Collection ID is required", http.StatusBadRequest)
		return
	}

	rows, err := db.Query(`
		SELECT post_id FROM collection_posts WHERE collection_id = ?`, id)
	if err != nil {
		httpError(w, "Unable to load collection", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var contentBuilder strings.Builder
	contentBuilder.WriteString(`<main>`)
	for rows.Next() {
		var postID int
		err := rows.Scan(&postID)
		if err != nil {
			httpError(w, "Unable to read post data", http.StatusInternalServerError)
			return
		}

		stmt, err := db.Prepare(`SELECT title, source, content, author, timestamp
			FROM posts WHERE id = ?`)
		if err != nil {
			httpError(w, "Unable to retrieve post", http.StatusInternalServerError)
			return
		}
		defer stmt.Close()

		var title, source, body, author, timestamp string
		err = stmt.QueryRow(postID).Scan(&title, &source, &body, &author, &timestamp)
		if err != nil {
			if err == sql.ErrNoRows {
				httpError(w, "Post not found", http.StatusNotFound)
				return
			}
			httpError(w, "Unable to retrieve post", http.StatusInternalServerError)
			return
		}

		uniqueID := fmt.Sprintf("%d", postID)

		if len(body) > 512 {
			contentBuilder.WriteString(string(post(uniqueID,
				title,
				source,
				fmt.Sprintf("%.*s...", 512, body),
				author,
				timestamp,
				"short")))
		} else {
			contentBuilder.WriteString(string(post(uniqueID,
				title,
				source,
				body,
				author,
				timestamp,
				"short")))
		}
	}

	err = rows.Err()
	if err != nil {
		httpError(w, "Unable to load collection", http.StatusInternalServerError)
		return
	}

	contentBuilder.WriteString(`</main>`)
	page, err := renderPage(contentBuilder.String())
	if err != nil {
		httpError(w, "Unable to load page", http.StatusInternalServerError)
		return
	}

	_, err = fmt.Fprintln(w, page)
	if err != nil {
		perror("Error writing response", err)
	}
}

func viewCollectionsHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT id, title, description, author, timestamp FROM collections
		ORDER BY timestamp DESC`)
	if err != nil {
		httpError(w, "Unable to load collections", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var contentBuilder strings.Builder
	contentBuilder.WriteString(`<main>
		<a href="/create-collection">Create Collection</a>
	`)

	for rows.Next() {
		var id int
		var title, description, author, timestamp string
		err := rows.Scan(&id, &title, &description, &author, &timestamp)
		if err != nil {
			httpError(w, "Unable to read collection data", http.StatusInternalServerError)
			return
		}

		uniqueID := fmt.Sprintf("%d", id)

		contentBuilder.WriteString(string(collection(uniqueID,
			title,
			description,
			author,
			timestamp,
			"short")))
	}

	err = rows.Err()
	if err != nil {
		httpError(w, "Unable to load collections", http.StatusInternalServerError)
		return
	}

	contentBuilder.WriteString(`</main>`)
	page, err := renderPage(contentBuilder.String())
	if err != nil {
		httpError(w, "Unable to load page", http.StatusInternalServerError)
		return
	}

	_, err = fmt.Fprintln(w, page)
	if err != nil {
		perror("Error writing response", err)
	}
}

func createCollectionHandler(w http.ResponseWriter, r *http.Request) {
	html := `<main>` + string(form("new-collection", `
	<label for="title">Title:</label>
	<input type="text" id="title" name="title" required><br>
	<label for="description">Description:</label>
	<textarea id="description" name="description" cols="80" rows="15"></textarea><br>
	<input type="submit" value="Submit"/>`)) + `</main>`

	page, err := renderPage(html)
	if err != nil {
		httpError(w, "Unable to load page", http.StatusInternalServerError)
		return
	}

	_, err = fmt.Fprintln(w, page)
	if err != nil {
		perror("Error writing response", err)
	}
}

func newCollectionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	title := template.HTMLEscapeString(r.FormValue("title"))
	description := template.HTMLEscapeString(r.FormValue("description"))
	author := template.HTMLEscapeString("Anonymous")
	timestamp := time.Now().UTC().Format(time.RFC3339)

	_, err := db.Exec(`INSERT INTO
		collections(title, description, author, timestamp)
		VALUES (?, ?, ?, ?)`, title, description, author, timestamp)
	if err != nil {
		httpError(w, "Unable to create collection", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/collections", http.StatusSeeOther)
}
