package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

func createPostHandler(w http.ResponseWriter, r *http.Request, errFunc httpErrorFunc) {
	keyword := r.FormValue("keyword")
	source := r.FormValue("source")

	var title, url, content string

	if source == "Wikipedia" {
		var err error
		title, url, content, err = fetchWikipediaContent(keyword)
		if err != nil {
			errFunc(w, "Error fetching Wikipedia article", http.StatusInternalServerError)
			return
		}
	} else if source == "Wikiquote" {
		var err error
		title, url, content, err = fetchWikiquoteContent(keyword)
		if err != nil {
			fmt.Println(err)
			errFunc(w, "Error fetching Wikiquote article", http.StatusInternalServerError)
			return
		}
	} else if source == "Link" {
		var err error
		title, url, content, err = fetchLinkContent(keyword)
		if err != nil {
			errFunc(w, "Error fetching Link content", http.StatusInternalServerError)
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
		errFunc(w, "Unable to load page", http.StatusInternalServerError)
		return
	}

	_, err = fmt.Fprintln(w, page)
	if err != nil {
		log.Printf("%s: %v", "Error writing response", err)
	}
}

func newPostHandler(w http.ResponseWriter, r *http.Request, errFunc httpErrorFunc) {
	if r.Method != http.MethodPost {
		errFunc(w, "Method not allowed", http.StatusMethodNotAllowed)
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
		errFunc(w, "Error processing text-to-speech", http.StatusInternalServerError)
		return
	}

	result, err := db.Exec(`INSERT INTO
		posts(title, source, content, author, timestamp)
		VALUES (?, ?, ?, ?, ?)`, title, source, content, author, timestamp)
	if err != nil {
		errFunc(w, "Error creating post", http.StatusInternalServerError)
		return
	}

	if collection != "" {
		lastInsertID, err := result.LastInsertId()
		if err != nil {
			errFunc(w, "Error getting last insert ID", http.StatusInternalServerError)
			return
		}

		_, err = db.Exec(`INSERT INTO
			collection_posts(collection_id, post_id)
			VALUES (?, ?)`, collection, lastInsertID)
		if err != nil {
			errFunc(w, "Error adding post to collection", http.StatusInternalServerError)
			return
		}
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func createCollectionHandler(w http.ResponseWriter, r *http.Request, errFunc httpErrorFunc) {
	html := `<main>` + string(form("new-collection", `
	<label for="title">Title:</label>
	<input type="text" id="title" name="title" required><br>
	<label for="description">Description:</label>
	<textarea id="description" name="description" cols="80" rows="15"></textarea><br>
	<input type="submit" value="Submit"/>`)) + `</main>`

	page, err := renderPage(html)
	if err != nil {
		errFunc(w, "Unable to load page", http.StatusInternalServerError)
		return
	}

	_, err = fmt.Fprintln(w, page)
	if err != nil {
		log.Printf("%s: %v", "Error writing response", err)
	}
}

func newCollectionHandler(w http.ResponseWriter, r *http.Request, errFunc httpErrorFunc) {
	title := template.HTMLEscapeString(r.FormValue("title"))
	description := template.HTMLEscapeString(r.FormValue("description"))
	author := template.HTMLEscapeString("Anonymous")
	timestamp := time.Now().UTC().Format(time.RFC3339)

	_, err := db.Exec(`INSERT INTO
		collections(title, description, author, timestamp)
		VALUES (?, ?, ?, ?)`, title, description, author, timestamp)
	if err != nil {
		errFunc(w, "Unable to create collection", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/collections", http.StatusSeeOther)
}
