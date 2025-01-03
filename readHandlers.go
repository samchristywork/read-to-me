package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"
)

func formatTime(ms int) string {
	seconds := ms / 1000
	minutes := seconds / 60
	hours := minutes / 60

	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes%60, seconds%60)
	} else if minutes > 0 {
		return fmt.Sprintf("%d:%02d", minutes, seconds%60)
	} else {
		return fmt.Sprintf("%d", seconds)
	}
}

func viewPostHandler(w http.ResponseWriter, r *http.Request, errFunc httpErrorFunc) {
	id := r.URL.Query().Get("id")
	if id == "" {
		errFunc(w, "Post ID is required", http.StatusBadRequest)
		return
	}

	stmt, err := db.Prepare(`SELECT title, source, content, author, timestamp
		FROM posts WHERE id = ?`)
	if err != nil {
		errFunc(w, "Unable to retrieve post", http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	var title, source, body, author, timestamp string
	err = stmt.QueryRow(id).Scan(&title, &source, &body, &author, &timestamp)
	if err != nil {
		if err == sql.ErrNoRows {
			errFunc(w, "Post not found", http.StatusNotFound)
			return
		}

		errFunc(w, "Unable to retrieve post", http.StatusInternalServerError)
		return
	}

	tmpl, err := templateFiles.ReadFile("template/audioControls.html")
	if err != nil {
		errFunc(w, "Unable to load page", http.StatusInternalServerError)
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
		errFunc(w, "Unable to load page", http.StatusInternalServerError)
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
		errFunc(w, "Unable to load page", http.StatusInternalServerError)
		return
	}

	_, err = fmt.Fprintln(w, page)
	if err != nil {
		log.Printf("%s: %v", "Error writing response", err)
	}
}

func viewPostsHandler(w http.ResponseWriter, r *http.Request, errFunc httpErrorFunc) {
	rows, err := db.Query(`
		SELECT id, title, source, content, author, timestamp FROM posts
		ORDER BY timestamp DESC`)
	if err != nil {
		errFunc(w, "Unable to load posts", http.StatusInternalServerError)
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
			errFunc(w, "Unable to read post data", http.StatusInternalServerError)
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
		errFunc(w, "Unable to load posts", http.StatusInternalServerError)
		return
	}

	contentBuilder.WriteString(`</main>`)

	page, err := renderPage(contentBuilder.String())
	if err != nil {
		errFunc(w, "Unable to load page", http.StatusInternalServerError)
		return
	}

	_, err = fmt.Fprintln(w, page)
	if err != nil {
		log.Printf("%s: %v", "Error writing response", err)
	}
}

func viewCollectionHandler(w http.ResponseWriter, r *http.Request, errFunc httpErrorFunc) {
	id := r.URL.Query().Get("id")
	if id == "" {
		errFunc(w, "Collection ID is required", http.StatusBadRequest)
		return
	}

	rows, err := db.Query(`
		SELECT post_id FROM collection_posts WHERE collection_id = ?`, id)
	if err != nil {
		errFunc(w, "Unable to load collection", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var contentBuilder strings.Builder
	contentBuilder.WriteString(`<main>`)
	for rows.Next() {
		var postID int
		err := rows.Scan(&postID)
		if err != nil {
			errFunc(w, "Unable to read post data", http.StatusInternalServerError)
			return
		}

		stmt, err := db.Prepare(`SELECT title, source, content, author, timestamp
			FROM posts WHERE id = ?`)
		if err != nil {
			errFunc(w, "Unable to retrieve post", http.StatusInternalServerError)
			return
		}
		defer stmt.Close()

		var title, source, body, author, timestamp string
		err = stmt.QueryRow(postID).Scan(&title, &source, &body, &author, &timestamp)
		if err != nil {
			if err == sql.ErrNoRows {
				errFunc(w, "Post not found", http.StatusNotFound)
				return
			}
			errFunc(w, "Unable to retrieve post", http.StatusInternalServerError)
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
		errFunc(w, "Unable to load collection", http.StatusInternalServerError)
		return
	}

	contentBuilder.WriteString(`</main>`)
	page, err := renderPage(contentBuilder.String())
	if err != nil {
		errFunc(w, "Unable to load page", http.StatusInternalServerError)
		return
	}

	_, err = fmt.Fprintln(w, page)
	if err != nil {
		log.Printf("%s: %v", "Error writing response", err)
	}
}

func viewCollectionsHandler(w http.ResponseWriter, r *http.Request, errFunc httpErrorFunc) {
	rows, err := db.Query(`
		SELECT id, title, description, author, timestamp FROM collections
		ORDER BY timestamp DESC`)
	if err != nil {
		errFunc(w, "Unable to load collections", http.StatusInternalServerError)
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
			errFunc(w, "Unable to read collection data", http.StatusInternalServerError)
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
		errFunc(w, "Unable to load collections", http.StatusInternalServerError)
		return
	}

	contentBuilder.WriteString(`</main>`)
	page, err := renderPage(contentBuilder.String())
	if err != nil {
		errFunc(w, "Unable to load page", http.StatusInternalServerError)
		return
	}

	_, err = fmt.Fprintln(w, page)
	if err != nil {
		log.Printf("%s: %v", "Error writing response", err)
	}
}

func audioHandler(w http.ResponseWriter, r *http.Request, errFunc httpErrorFunc) {
	id := r.URL.Query().Get("id")
	if id == "" {
		errFunc(w, "Post ID is required", http.StatusBadRequest)
		return
	}

	stmt, err := db.Prepare("SELECT content FROM posts WHERE id = ?")
	if err != nil {
		errFunc(w, "Unable to retrieve post", http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	var body string
	err = stmt.QueryRow(id).Scan(&body)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Post not found", http.StatusNotFound)
			return
		}
		errFunc(w, "Unable to retrieve post", http.StatusInternalServerError)
		return
	}

	var audioBytes [][]byte

	lines := strings.Split(body, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var audioContent []byte
		audioContent, _, err = retrieveTTS(line)
		if err != nil {
			errFunc(w, "Unable to retrieve audio", http.StatusInternalServerError)
			return
		}

		audioBytes = append(audioBytes, audioContent)
	}

	fullAudio := bytes.Join(audioBytes, []byte(""))

	http.ServeContent(w, r, "audio.mp3", time.Now(), bytes.NewReader(fullAudio))
}
