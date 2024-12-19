package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	texttospeech "cloud.google.com/go/texttospeech/apiv1"
	"cloud.google.com/go/texttospeech/apiv1/texttospeechpb"
	_ "github.com/mattn/go-sqlite3"
	"github.com/tcolgate/mp3"
	"golang.org/x/text/unicode/norm"
)

var dbMutex sync.Mutex

//go:embed template/*
var templateFiles embed.FS

var db *sql.DB

func calculateAudioLength(audioContent []byte) (int, error) {
	reader := bytes.NewReader(audioContent)
	decoder := mp3.NewDecoder(reader)

	var frame mp3.Frame
	var skipped int
	var totalDuration time.Duration

	for {
		if err := decoder.Decode(&frame, &skipped); err != nil {
			break
		}
		totalDuration += frame.Duration()
	}

	return int(totalDuration.Milliseconds()), nil
}

func calculateHash(text string) string {
	hasher := sha256.New()
	normalizedText := norm.NFC.String(text)
	hasher.Write([]byte(normalizedText))
	return hex.EncodeToString(hasher.Sum(nil))
}

func generateTTS(text string) (string, string, []byte, int, error) {
	audioHash := calculateHash(text)

	audioContent, audioLength, err := retrieveTTS(text)
	if err != nil {
		log.Printf("Error checking existing audio: %v", err)
		return "", "", nil, 0, err
	}

	if audioContent != nil {
		log.Printf("Audio for text already exists, using cached version.")
		return audioHash, text, audioContent, audioLength, nil
	}

	ctx := context.Background()
	client, err := texttospeech.NewClient(ctx)
	if err != nil {
		return "", "", nil, 0, err
	}
	defer client.Close()

	req := texttospeechpb.SynthesizeSpeechRequest{
		Input: &texttospeechpb.SynthesisInput{
			InputSource: &texttospeechpb.SynthesisInput_Text{Text: text},
		},
		Voice: &texttospeechpb.VoiceSelectionParams{
			LanguageCode: "en-US",
			SsmlGender:   texttospeechpb.SsmlVoiceGender_MALE,
		},
		AudioConfig: &texttospeechpb.AudioConfig{
			AudioEncoding: texttospeechpb.AudioEncoding_MP3,
		},
	}

	log.Printf("Synthesizing: %.*s\n", 40, text)

	resp, err := client.SynthesizeSpeech(ctx, &req)
	if err != nil {
		_, t, c, l, _ := generateTTS("Text could not be synthesized.")
		_, err = db.Exec(`INSERT INTO audio (hash, text, audio, audio_length_ms)
			VALUES (?, ?, ?, ?)`, audioHash, t, c, l)
		log.Printf("Error synthesizing text: %v", err)
		return "", "", nil, 0, err
	}

	audioContent = resp.AudioContent
	audioLength, err = calculateAudioLength(audioContent)
	if err != nil {
		log.Printf("Error calculating audio length: %v", err)
		return "", "", nil, 0, err
	}

	log.Printf("Inserting into audio table: hash=%s, text=%s, audio_length_ms=%d",
		audioHash, text, audioLength)

	dbMutex.Lock()
	_, err = db.Exec(`INSERT INTO audio (hash, text, audio, audio_length_ms)
	VALUES (?, ?, ?, ?)`, audioHash, text, audioContent, audioLength)
	dbMutex.Unlock()

	if err != nil {
		log.Printf("Error saving audio cache: %v", err)
		return "", "", nil, 0, err
	}

	return audioHash, text, audioContent, audioLength, nil
}

func post(id, title, url, content, author, time, class string) template.HTML {
	return template.HTML(fmt.Sprintf(`
		<div class="post %s">
			<h3><a href="/post?id=%s">%s</a> - <a href="%s">%s</a></h3>
			<em data-timestamp="%s">%s at %s</em>
			<div class="post-body">%s</div>
		</div>
	`, class, id, title, url, "source", time, author, time, content))
}

func retrieveTTS(text string) ([]byte, int, error) {
	audioHash := calculateHash(text)
	var audioContent []byte
	var audioLength int

	dbMutex.Lock()
	err := db.QueryRow("SELECT audio, audio_length_ms FROM audio WHERE hash = ?",
		audioHash).Scan(&audioContent, &audioLength)
	dbMutex.Unlock()

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, 0, nil
		}
		log.Printf("Error fetching audio cache: %v", err)
		return nil, 0, err
	}

	return audioContent, audioLength, nil
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
		http.Error(w, "Internal Server Error: Unable to load page",
			http.StatusInternalServerError)
		return
	}

	if _, err := fmt.Fprintln(w, page); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

func form(action, content string) template.HTML {
	return template.HTML(fmt.Sprintf(`<form method="post" action="%s">%s</form>`,
		action, content))
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT id, title, source, content, author, timestamp FROM posts
		ORDER BY timestamp DESC`)
	if err != nil {
		log.Printf("Error querying posts: %v", err)
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
			log.Printf("Error scanning row: %v", err)
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
		log.Printf("Error after iterating rows: %v", err)
		http.Error(w, "Internal Server Error: Unable to load posts",
			http.StatusInternalServerError)
		return
	}

	contentBuilder.WriteString(`
	<script>
		document.querySelectorAll(".post em").forEach((e) => {
			const timestamp = e.getAttribute("data-timestamp");
			const localDate = new Date(timestamp).toLocaleString(undefined, {
				timeZoneName: "short"
			});
			e.textContent = e.textContent.split(" at ")[0] + " at " + localDate;
		});
	</script>
	</main>`)

	page, err := renderPage(contentBuilder.String())
	if err != nil {
		http.Error(w, "Internal Server Error: Unable to load page",
			http.StatusInternalServerError)
		return
	}

	if _, err := fmt.Fprintln(w, page); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

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

func viewHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Post ID is required", http.StatusBadRequest)
		return
	}

	stmt, err := db.Prepare(`SELECT title, source, content, author, timestamp
		FROM posts WHERE id = ?`)
	if err != nil {
		log.Printf("Error preparing SQL statement: %v", err)
		http.Error(w, "Internal Server Error: Unable to retrieve post",
			http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	var title, source, body, author, timestamp string
	err = stmt.QueryRow(id).Scan(&title, &source, &body, &author, &timestamp)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Post not found", http.StatusNotFound)
			return
		}
		log.Printf("Error querying post: %v", err)
		http.Error(w, "Internal Server Error: Unable to retrieve post",
			http.StatusInternalServerError)
		return
	}

	tmpl, err := templateFiles.ReadFile("template/audioControls.html")
	if err != nil {
		log.Printf("Error reading head template: %v", err)
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

		log.Printf("Searching for hash %s, %s", audioHash, line)
		err := db.QueryRow("SELECT audio_length_ms FROM audio WHERE hash = ?",
			audioHash).Scan(&audioLength)
		if err != nil {
			if err != sql.ErrNoRows {
				log.Printf("Error querying audio length: %v", err)
			}
			log.Printf("Could not get audio length: %v", err)
			continue
		}

		content += fmt.Sprintf(`<span class="hidden">%d</span>`, audioLength)

		fmt.Printf("Audio Length: %d %.*s\n", audioLength, 40, line)
		totalLength += audioLength
	}

	navigation += "</div>"

	fmt.Printf("Total length of all lines (in ms): %d\n", totalLength)

	script, err := templateFiles.ReadFile("template/script.js")
	if err != nil {
		http.Error(w, "Internal Server Error: Unable to load page",
			http.StatusInternalServerError)
		return
	}

	content = `<main>` +
		string(post(id, title, source, content, author, timestamp, "full")) +
		audio + navigation + `</main>` + string(script)

	page, err := renderPage(content)
	if err != nil {
		http.Error(w, "Internal Server Error: Unable to load page",
			http.StatusInternalServerError)
		return
	}

	_, err = fmt.Fprintln(w, page)
	if err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

func audioHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Post ID is required", http.StatusBadRequest)
		return
	}

	stmt, err := db.Prepare("SELECT content FROM posts WHERE id = ?")
	if err != nil {
		log.Printf("Error preparing SQL statement: %v", err)
		http.Error(w, "Internal Server Error: Unable to retrieve post",
			http.StatusInternalServerError)
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
		log.Printf("Error querying post: %v", err)
		http.Error(w, "Internal Server Error: Unable to retrieve post",
			http.StatusInternalServerError)
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
			log.Printf("Error retrieving audio cache for line %s\n: %v", line, err)
			http.Error(w, "Internal Server Error: Unable to retrieve audio",
				http.StatusInternalServerError)
			return
		}

		audioBytes = append(audioBytes, audioContent)
	}

	fullAudio := bytes.Join(audioBytes, []byte(""))

	http.ServeContent(w, r, "audio.mp3", time.Now(), bytes.NewReader(fullAudio))
}

func editHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Post ID is required", http.StatusBadRequest)
		return
	}

	stmt, err := db.Prepare(`SELECT title, content, author, source, timestamp
		FROM posts WHERE id = ?`)
	if err != nil {
		log.Printf("Error preparing SQL statement: %v", err)
		http.Error(w, "Internal Server Error: Unable to retrieve post",
			http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	var title, body, author, source, timestamp string
	err = stmt.QueryRow(id).Scan(&title, &body, &author, &source, &timestamp)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Post not found", http.StatusNotFound)
			return
		}
		log.Printf("Error querying post: %v", err)
		http.Error(w, "Internal Server Error: Unable to retrieve post",
			http.StatusInternalServerError)
		return
	}

	content := createPage(title, source, body)
	page, err := renderPage(content)

	if err != nil {
		http.Error(w, "Internal Server Error: Unable to load page",
			http.StatusInternalServerError)
		return
	}

	if _, err := fmt.Fprintln(w, page); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

func createPage(title, source, body string) string {
	title = strings.ReplaceAll(title, `"`, "")
	source = strings.ReplaceAll(source, `"`, "")
	body = strings.ReplaceAll(body, `"`, "")

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
			<label for="source">Source:</label>
			<input type="url" id="source" name="source" placeholder="https://example.com" value="`+source+`"><br>
			<label for="content">Body:</label>
			<textarea id="content" name="content" cols="80" rows="15" required>`+body+`</textarea><br>
			<input type="submit" value="Submit"/>
		`)),
		string(form("create", `
			<label for="keyword">Wikipedia:</label>
			<input type="text" id="keyword" name="keyword" required><br>
			<input type="hidden" id="source" name="source" value="Wikipedia">
			<input type="submit" value="Search"/>
		`)),
		string(form("create", `
			<label for="keyword">Link:</label>
			<input type="text" id="keyword" name="keyword" required><br>
			<input type="hidden" id="source" name="source" value="Link">
			<input type="submit" value="Search"/>
		`)),
	)
}

func createHandler(w http.ResponseWriter, r *http.Request) {
	keyword := r.FormValue("keyword")
	source := r.FormValue("source")

	var title, url, content string

	if source == "Wikipedia" {
		var err error
		title, url, content, err = fetchWikipediaContent(keyword)
		if err != nil {
			log.Println(err)
			http.Error(w, "Error fetching Wikipedia article",
				http.StatusInternalServerError)
			return
		}
	} else if source == "Link" {
		var err error
		title, url, content, err = fetchLinkContent(keyword)
		if err != nil {
			log.Println(err)
			http.Error(w, "Error fetching Link content",
				http.StatusInternalServerError)
			return
		}
	} else {
		title = ""
		url = ""
		content = ""
	}

	pageContent := createPage(title, url, content)
	page, err := renderPage(pageContent)
	if err != nil {
		http.Error(w, "Internal Server Error: Unable to load page",
			http.StatusInternalServerError)
		return
	}

	if _, err := fmt.Fprintln(w, page); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

func newPostHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	title := template.HTMLEscapeString(r.FormValue("title"))
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
				log.Printf("Error processing line %s\n: %v", text, err)
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

	if ttsError != nil && !strings.Contains(ttsError.Error(), "UNIQUE constraint failed") {
		http.Error(w, "Error processing text-to-speech: "+ttsError.Error(),
			http.StatusInternalServerError)
		return
	}

	_, err := db.Exec(`INSERT INTO
		posts(title, source, content, author, timestamp)
		VALUES (?, ?, ?, ?, ?)`, title, source, content, author, timestamp)
	if err != nil {
		http.Error(w, "Error creating post: "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
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

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS collections (
		id INTEGER PRIMARY KEY,
		title TEXT,
		description TEXT,
		author TEXT,
		timestamp TEXT
	)`)
	if err != nil {
		log.Fatalf("Error creating posts table: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS collection_posts (
		collection_id INTEGER,
		post_id INTEGER
	)`)
	if err != nil {
		log.Fatalf("Error creating collection_posts table: %v", err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			homeHandler(w, r)
		case "/post":
			viewHandler(w, r)
		case "/audio":
			audioHandler(w, r)
		case "/edit":
			editHandler(w, r)
		case "/create":
			createHandler(w, r)
		case "/new-post":
			newPostHandler(w, r)
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
