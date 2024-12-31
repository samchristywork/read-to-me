package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

func viewPostHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		httpError(w, "Post ID is required", http.StatusBadRequest)
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
	err = stmt.QueryRow(id).Scan(&title, &source, &body, &author, &timestamp)
	if err != nil {
		if err == sql.ErrNoRows {
			httpError(w, "Post not found", http.StatusNotFound)
			return
		}

		httpError(w, "Unable to retrieve post", http.StatusInternalServerError)
		return
	}

	tmpl, err := templateFiles.ReadFile("template/audioControls.html")
	if err != nil {
		httpError(w, "Unable to load page", http.StatusInternalServerError)
		return
	}
	audioControls := string(tmpl)

	audio := fmt.Sprintf(`<div class="audio-container">
	<audio id="audioPlayer" controls="controls" autobuffer="autobuffer">
		<source src="/audio?id=%s&t=%d" />
	</audio>
	%s
	<label for="autoscroll">Autoscroll:</label>
	<input id="autoscroll" name="autoscroll" type=checkbox checked></input>
	</div>`, id, time.Now().Unix(), audioControls)
	// TODO: Remove the anti-caching later.

	navigation := `<div class="navigation">`

	content := ""
	lines := strings.Split(body, "\n")

	totalLength := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)

		if line == "" {
			content += "<br>"
			continue
		}

		content += "<div>" + template.HTMLEscapeString(line) + "</div>"
		epsilon := 1
		navigation += `
		<div onclick="document.getElementById('audioPlayer').currentTime = ` +
			fmt.Sprintf("%f", float64(totalLength+epsilon)/1000) + `;">` +
			`<span style="text-align:right;">` + formatTime(totalLength) + `</span>` +
			`<span>` + template.HTMLEscapeString(line) + `</span>` +
			`</div>`

		var audioLength int
		audioHash := calculateHash(line)

		err := db.QueryRow("SELECT audio_length_ms FROM audio WHERE hash = ?",
			audioHash).Scan(&audioLength)
		if err != nil {
			log.Printf("%s: %v", "Error querying audio length", err)
			continue
		}

		content += fmt.Sprintf(`<span class="hidden">%d</span>`, audioLength)

		totalLength += audioLength
	}

	navigation += "</div>"

	script, err := templateFiles.ReadFile("template/script.js")
	if err != nil {
		httpError(w, "Unable to load page", http.StatusInternalServerError)
		return
	}

	scripts := string(script) + `<script>
		document.addEventListener("keydown", function(event) {
			if (event.key === " ") {
				var audio = document.getElementById("audioPlayer");
				event.preventDefault();
				if (audio.paused) {
					audio.play();
				} else {
					audio.pause();
				}
			}
		});
	</script>`

	content = `<main>` +
		string(post(id, title, source, content, author, timestamp, "full")) +
		audio + navigation + `</main>` + scripts

	page, err := renderPage(content)
	if err != nil {
		httpError(w, "Unable to load page", http.StatusInternalServerError)
		return
	}

	_, err = fmt.Fprintln(w, page)
	if err != nil {
		log.Printf("%s: %v", "Error writing response", err)
	}
}

func viewPostsHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT id, title, source, content, author, timestamp FROM posts
		ORDER BY timestamp DESC`)
	if err != nil {
		httpError(w, "Unable to load posts", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var contentBuilder strings.Builder
	contentBuilder.WriteString(`<main>`)

	for rows.Next() {
		var id int
		var title, source, body, author, timestamp string
		err := rows.Scan(&id, &title, &source, &body, &author, &timestamp)
		if err != nil {
			httpError(w, "Unable to read post data", http.StatusInternalServerError)
			return
		}

		uniqueID := fmt.Sprintf("%d", id)
		body = template.HTMLEscapeString(body)

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
		httpError(w, "Unable to load posts", http.StatusInternalServerError)
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
		log.Printf("%s: %v", "Error writing response", err)
	}
}

func editPostHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		httpError(w, "Post ID is required", http.StatusBadRequest)
		return
	}

	stmt, err := db.Prepare(`SELECT title, content, author, source, timestamp
		FROM posts WHERE id = ?`)
	if err != nil {
		httpError(w, "Unable to retrieve post", http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	var title, body, author, source, timestamp string
	err = stmt.QueryRow(id).Scan(&title, &body, &author, &source, &timestamp)
	if err != nil {
		if err == sql.ErrNoRows {
			httpError(w, "Post not found", http.StatusNotFound)
			return
		}
		httpError(w, "Unable to retrieve post", http.StatusInternalServerError)
		return
	}

	content := createPost(title, source, body)
	page, err := renderPage(content)

	if err != nil {
		httpError(w, "Unable to load page", http.StatusInternalServerError)
		return
	}

	_, err = fmt.Fprintln(w, page)
	if err != nil {
		log.Printf("%s: %v", "Error writing response", err)
	}
}

func createPostHandler(w http.ResponseWriter, r *http.Request) {
	keyword := r.FormValue("keyword")
	source := r.FormValue("source")

	var title, url, content string

	if source == "Wikipedia" {
		var err error
		title, url, content, err = fetchWikipediaContent(keyword)
		if err != nil {
			httpError(w, "Error fetching Wikipedia article", http.StatusInternalServerError)
			return
		}
	} else if source == "Wikiquote" {
		var err error
		title, url, content, err = fetchWikiquoteContent(keyword)
		if err != nil {
			fmt.Println(err)
			httpError(w, "Error fetching Wikiquote article", http.StatusInternalServerError)
			return
		}
	} else if source == "Link" {
		var err error
		title, url, content, err = fetchLinkContent(keyword)
		if err != nil {
			httpError(w, "Error fetching Link content", http.StatusInternalServerError)
			return
		}
	} else {
		title = ""
		url = ""
		content = ""
	}

	pageContent := createPost(title, url, content)
	page, err := renderPage(pageContent)
	if err != nil {
		httpError(w, "Unable to load page", http.StatusInternalServerError)
		return
	}

	_, err = fmt.Fprintln(w, page)
	if err != nil {
		log.Printf("%s: %v", "Error writing response", err)
	}
}

func newPostHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	title := template.HTMLEscapeString(r.FormValue("title"))
	collection := template.HTMLEscapeString(r.FormValue("collection"))
	source := template.HTMLEscapeString(r.FormValue("source"))
	content := r.FormValue("content")
	author := template.HTMLEscapeString("Anonymous")
	timestamp := time.Now().UTC().Format(time.RFC3339)

	fmt.Println(title, collection, source, content, author, timestamp)

	linesSet := make(map[string]struct{})
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		linesSet[line] = struct{}{}
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(linesSet))

	for line := range linesSet {
		wg.Add(1)
		go func(text string) {
			defer wg.Done()
			_, _, _, _, err := generateTTS(text)
			if err != nil {
				log.Printf("%s: %v", "Error processing line", err)
				errChan <- err
			}
		}(line)
	}

	go func() {
		wg.Wait()
		close(errChan)
	}()

	var ttsError error
	for err := range errChan {
		if err != nil {
			ttsError = err
			break
		}
	}

	// TODO: Fix this
	if ttsError != nil && !strings.Contains(ttsError.Error(), "UNIQUE constraint failed") {
		httpError(w, "Error processing text-to-speech", http.StatusInternalServerError)
		return
	}

	result, err := db.Exec(`INSERT INTO
		posts(title, source, content, author, timestamp)
		VALUES (?, ?, ?, ?, ?)`, title, source, content, author, timestamp)
	if err != nil {
		httpError(w, "Error creating post", http.StatusInternalServerError)
		return
	}

	if collection != "" {
		lastInsertID, err := result.LastInsertId()
		if err != nil {
			httpError(w, "Error getting last insert ID", http.StatusInternalServerError)
			return
		}

		_, err = db.Exec(`INSERT INTO
			collection_posts(collection_id, post_id)
			VALUES (?, ?)`, collection, lastInsertID)
		if err != nil {
			httpError(w, "Error adding post to collection", http.StatusInternalServerError)
			return
		}
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
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
		log.Printf("%s: %v", "Error writing response", err)
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
		log.Printf("%s: %v", "Error writing response", err)
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
		log.Printf("%s: %v", "Error writing response", err)
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
