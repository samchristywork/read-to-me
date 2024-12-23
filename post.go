package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"sync"
	"time"
)

func post(id, title, url, content, author, time, class string) template.HTML {
	return template.HTML(fmt.Sprintf(`
		<div class="post %s">
			<h3><a href="/post?id=%s">%s</a> - <a href="%s">%s</a></h3>
			<em data-timestamp="%s">%s at %s</em>
			<div class="post-body">%s</div>
		</div>
	`, class, id, title, url, "source", time, author, time, content))
}

func viewPostHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Post ID is required", http.StatusBadRequest)
		return
	}

	stmt, err := db.Prepare(`SELECT title, source, content, author, timestamp
		FROM posts WHERE id = ?`)
	if err != nil {
		perror("Error preparing SQL statement", err)
		http.Error(w, "Internal Server Error: Unable to retrieve post",
			http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	var title, source, body, author, timestamp string
	err = stmt.QueryRow(id).Scan(&title, &source, &body, &author, &timestamp)
	if err != nil {
		if err == sql.ErrNoRows {
			perror("Post not found", err)
			http.Error(w, "Post not found", http.StatusNotFound)
			return
		}

		perror("Error querying post", err)
		http.Error(w, "Internal Server Error: Unable to retrieve post",
			http.StatusInternalServerError)
		return
	}

	tmpl, err := templateFiles.ReadFile("template/audioControls.html")
	if err != nil {
		perror("Error reading head template", err)
		http.Error(w, "Internal Server Error: Unable to load page",
			http.StatusInternalServerError)
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

		//log.Printf("Searching for hash %s, %s", audioHash, line)
		err := db.QueryRow("SELECT audio_length_ms FROM audio WHERE hash = ?",
			audioHash).Scan(&audioLength)
		if err != nil {
			perror("Error querying audio length", err)
			continue
		}

		content += fmt.Sprintf(`<span class="hidden">%d</span>`, audioLength)

		//fmt.Printf("Audio Length: %d %.*s\n", audioLength, 40, line)
		totalLength += audioLength
	}

	navigation += "</div>"

	//fmt.Printf("Total length of all lines (in ms): %d\n", totalLength)
	script, err := templateFiles.ReadFile("template/script.js")
	if err != nil {
		perror("Error reading script template", err)
		http.Error(w, "Internal Server Error: Unable to load page",
			http.StatusInternalServerError)
		return
	}

	content = `<main>` +
		string(post(id, title, source, content, author, timestamp, "full")) +
		audio + navigation + `</main>` + string(script)

	page, err := renderPage(content)
	if err != nil {
		perror("Error rendering page", err)
		http.Error(w, "Internal Server Error: Unable to load page",
			http.StatusInternalServerError)
		return
	}

	_, err = fmt.Fprintln(w, page)
	if err != nil {
		perror("Error writing response", err)
	}
}

func viewPostsHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT id, title, source, content, author, timestamp FROM posts
		ORDER BY timestamp DESC`)
	if err != nil {
		perror("Error querying posts", err)
		http.Error(w, "Internal Server Error: Unable to load posts",
			http.StatusInternalServerError)
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
			perror("Error scanning row", err)
			http.Error(w, "Internal Server Error: Unable to read post data",
				http.StatusInternalServerError)
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
		perror("Error after iterating rows", err)
		http.Error(w, "Internal Server Error: Unable to load posts",
			http.StatusInternalServerError)
		return
	}

	contentBuilder.WriteString(`</main>`)

	page, err := renderPage(contentBuilder.String())
	if err != nil {
		perror("Error rendering page", err)
		http.Error(w, "Internal Server Error: Unable to load page",
			http.StatusInternalServerError)
		return
	}

	if _, err := fmt.Fprintln(w, page); err != nil {
		perror("Error writing response", err)
	}
}

func createPost(title, source, body string) string {
	title = strings.ReplaceAll(title, `"`, "")
	source = strings.ReplaceAll(source, `"`, "")
	body = strings.ReplaceAll(body, `"`, "")

	var collectionDropdown string
	rows, err := db.Query(`SELECT id, title FROM collections WHERE author = "Anonymous"`)
	if err != nil {
		perror("Error querying collections", err)
	} else {
		defer rows.Close()

		collectionDropdown = `<label for="collection">Collection:</label>
		<select id="collection" name="collection">
			<option value="">None</option>`
		for rows.Next() {
			var id int
			var title string
			err := rows.Scan(&id, &title)
			if err != nil {
				perror("Error scanning row", err)
				continue
			}
			collectionDropdown += fmt.Sprintf(`<option value="%d">%s</option>`, id, title)
		}
		collectionDropdown += `</select><br>`
	}

	return fmt.Sprintf(`
		<main>
			%s
			<div class="datasources">
				<div class="datasource">%s</div>
				<div class="datasource">%s</div>
			</div>
		</main>`,
		string(form("new-post", `
			<label for="title">Title:</label>
			<input type="text" id="title" name="title" value="`+title+`" autofocus required><br>
			`+collectionDropdown+`
			<label for="source">Source:</label>
			<input type="url" id="source" name="source" placeholder="https://example.com" value="`+source+`"><br>
			<label for="content">Body:</label>
			<textarea id="content" name="content" cols="80" rows="15" required>`+body+`</textarea><br>
			<input type="submit" value="Submit"/>
		`)),
		string(form("create-post", `
			<label for="keyword">Wikipedia:</label>
			<input type="text" id="keyword" name="keyword" required><br>
			<input type="hidden" id="source" name="source" value="Wikipedia">
			<input type="submit" value="Search"/>
		`)),
		string(form("create-post", `
			<label for="keyword">Link:</label>
			<input type="text" id="keyword" name="keyword" required><br>
			<input type="hidden" id="source" name="source" value="Link">
			<input type="submit" value="Search"/>
		`)),
	)
}

func editPostHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		perror("Post ID is required", nil)
		http.Error(w, "Post ID is required", http.StatusBadRequest)
		return
	}

	stmt, err := db.Prepare(`SELECT title, content, author, source, timestamp
		FROM posts WHERE id = ?`)
	if err != nil {
		perror("Error preparing SQL statement", err)
		http.Error(w, "Internal Server Error: Unable to retrieve post",
			http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	var title, body, author, source, timestamp string
	err = stmt.QueryRow(id).Scan(&title, &body, &author, &source, &timestamp)
	if err != nil {
		if err == sql.ErrNoRows {
			perror("Post not found", err)
			http.Error(w, "Post not found", http.StatusNotFound)
			return
		}
		perror("Error querying post", err)
		http.Error(w, "Internal Server Error: Unable to retrieve post",
			http.StatusInternalServerError)
		return
	}

	content := createPost(title, source, body)
	page, err := renderPage(content)

	if err != nil {
		perror("Error rendering page", err)
		http.Error(w, "Internal Server Error: Unable to load page",
			http.StatusInternalServerError)
		return
	}

	if _, err := fmt.Fprintln(w, page); err != nil {
		perror("Error writing response", err)
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
			perror("Error fetching Wikipedia article", err)
			http.Error(w, "Error fetching Wikipedia article",
				http.StatusInternalServerError)
			return
		}
	} else if source == "Link" {
		var err error
		title, url, content, err = fetchLinkContent(keyword)
		if err != nil {
			perror("Error fetching Link content", err)
			http.Error(w, "Error fetching Link content",
				http.StatusInternalServerError)
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
		perror("Error rendering page", err)
		http.Error(w, "Internal Server Error: Unable to load page",
			http.StatusInternalServerError)
		return
	}

	if _, err := fmt.Fprintln(w, page); err != nil {
		perror("Error writing response", err)
	}
}

func newPostHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		perror("Method not allowed", nil)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	title := template.HTMLEscapeString(r.FormValue("title"))
	collection := template.HTMLEscapeString(r.FormValue("collection"))
	source := template.HTMLEscapeString(r.FormValue("source"))
	content := r.FormValue("content")
	author := template.HTMLEscapeString("Anonymous")
	timestamp := time.Now().UTC().Format(time.RFC3339)

	lines := strings.Split(content, "\n")
	var wg sync.WaitGroup
	errChan := make(chan error, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		wg.Add(1)
		go func(text string) {
			defer wg.Done()
			_, _, _, _, err := generateTTS(text)
			if err != nil {
				perror("Error processing line", err)
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
		perror("Error processing text-to-speech", ttsError)
		http.Error(w, "Error processing text-to-speech: "+ttsError.Error(),
			http.StatusInternalServerError)
		return
	}

	result, err := db.Exec(`INSERT INTO
		posts(title, source, content, author, timestamp)
		VALUES (?, ?, ?, ?, ?)`, title, source, content, author, timestamp)
	if err != nil {
		perror("Error creating post", err)
		http.Error(w, "Error creating post: "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	if collection != "" {
		lastInsertID, err := result.LastInsertId()
		if err != nil {
			perror("Error getting last insert ID", err)
			http.Error(w, "Error getting last insert ID: "+err.Error(),
				http.StatusInternalServerError)
			return
		}

		_, err = db.Exec(`INSERT INTO
			collection_posts(collection_id, post_id)
			VALUES (?, ?)`, collection, lastInsertID)
		if err != nil {
			perror("Error adding post to collection", err)
			http.Error(w, "Error adding post to collection: "+err.Error(),
				http.StatusInternalServerError)
			return
		}
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}
