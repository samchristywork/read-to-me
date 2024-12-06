package main

import (
	"net/http"
	"fmt"
	"log"
	"crypto/sha256"
	"database/sql"
	"os"
	"embed"
	"bytes"
	"time"
	"sync"
	"strings"
	"encoding/hex"
	"html/template"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/text/unicode/norm"
)

var dbMutex sync.Mutex

//go:embed template/*
var templateFiles embed.FS

var db *sql.DB

func calculateHash(text string) string {
	hasher := sha256.New()
	normalizedText := norm.NFC.String(text)
	hasher.Write([]byte(normalizedText))
	return hex.EncodeToString(hasher.Sum(nil))
}

func post(id, title, url, content, author, timestamp, class string) template.HTML {
	return template.HTML(fmt.Sprintf(`
		<div class="post %s">
			<h3><a href="/post?id=%s">%s</a> - <a href="%s">%s</a></h3>
			<em data-timestamp="%s">%s at %s</em>
			<div class="post-body">%s</div>
		</div>
	`, class, id, title, url, "source", timestamp, author, timestamp, content))
}

func retrieveTTS(text string) ([]byte, int, error) {
	audioHash := calculateHash(text)
	var audioContent []byte
	var audioLength int

	dbMutex.Lock()
	err := db.QueryRow("SELECT audio, audio_length_ms FROM audio WHERE hash = ?", audioHash).Scan(&audioContent, &audioLength)
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
		http.Error(w, "Internal Server Error: Unable to load page", http.StatusInternalServerError)
		return
	}

	if _, err := fmt.Fprintln(w, page); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

func form(action, content string) template.HTML {
	return template.HTML(fmt.Sprintf(`<form method="post" action="%s">%s</form>`, action, content))
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

		if len(body) > 50 {
			contentBuilder.WriteString(string(post(uniqueID, title, source, fmt.Sprintf("%.*s...", 512, body), author, timestamp, "short")))
		} else {
			contentBuilder.WriteString(string(post(uniqueID, title, source, body, author, timestamp, "short")))
		}
	}

	if err := rows.Err(); err != nil {
		log.Printf("Error after iterating rows: %v", err)
		http.Error(w, "Internal Server Error: Unable to load posts", http.StatusInternalServerError)
		return
	}

	contentBuilder.WriteString(`
	<script>
		document.querySelectorAll(".post em").forEach((element) => {
			const timestamp = element.getAttribute("data-timestamp");
			const localDate = new Date(timestamp).toLocaleString(undefined, { timeZoneName: "short" });
			element.textContent = element.textContent.split(" at ")[0] + " at " + localDate;
		});
	</script>
	</main>`)

	page, err := renderPage(contentBuilder.String())
	if err != nil {
		http.Error(w, "Internal Server Error: Unable to load page", http.StatusInternalServerError)
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

	stmt, err := db.Prepare("SELECT title, source, content, author, timestamp FROM posts WHERE id = ?")
	if err != nil {
		log.Printf("Error preparing SQL statement: %v", err)
		http.Error(w, "Internal Server Error: Unable to retrieve post", http.StatusInternalServerError)
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
		http.Error(w, "Internal Server Error: Unable to retrieve post", http.StatusInternalServerError)
		return
	}

	audio := fmt.Sprintf(`<div class="audio-container">
	<audio id="audioPlayer" controls="controls" autobuffer="autobuffer">
		<source src="/audio?id=%s&foo=bar" />
	</audio>
	<div>
		<button onclick="document.getElementById('audioPlayer').playbackRate = 0.5;">0.5x</button>
		<button onclick="document.getElementById('audioPlayer').playbackRate = 1;">1x</button>
		<button onclick="document.getElementById('audioPlayer').playbackRate = 1.5;">1.5x</button>
		<button onclick="document.getElementById('audioPlayer').playbackRate = 2;">2x</button>
	</div>
	<label for="autoscroll">Autoscroll:</label>
	<input id="autoscroll" name="autoscroll" type=checkbox checked></input>
	</div>`, id)

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
		<div onclick="document.getElementById('audioPlayer').currentTime = ` + fmt.Sprintf("%f", float64(totalLength+epsilon)/1000) + `;">` +
			`<span style="text-align:right;">` + formatTime(totalLength) + `</span>` +
			`<span>` + template.HTMLEscapeString(line) + `</span>` +
			`</div>`

		var audioLength int
		audioHash := calculateHash(line)

		log.Printf("Searching for hash %s, %s", audioHash, line)
		err := db.QueryRow("SELECT audio_length_ms FROM audio WHERE hash = ?", audioHash).Scan(&audioLength)
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
		http.Error(w, "Internal Server Error: Unable to load page", http.StatusInternalServerError)
		return
	}

	content = `<main>` + string(post(id, title, source, content, author, timestamp, "full")) + audio + navigation + `</main>` + string(script)

	page, err := renderPage(content)
	if err != nil {
		http.Error(w, "Internal Server Error: Unable to load page", http.StatusInternalServerError)
		return
	}

	if _, err := fmt.Fprintln(w, page); err != nil {
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
		http.Error(w, "Internal Server Error: Unable to retrieve post", http.StatusInternalServerError)
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
		http.Error(w, "Internal Server Error: Unable to retrieve post", http.StatusInternalServerError)
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
			http.Error(w, "Internal Server Error: Unable to retrieve audio", http.StatusInternalServerError)
			return
		}

		audioBytes = append(audioBytes, audioContent)
	}

	fullAudio := bytes.Join(audioBytes, []byte(""))

	http.ServeContent(w, r, "audio.mp3", time.Now(), bytes.NewReader(fullAudio))
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
		case "/post":
			viewHandler(w, r)
		case "/audio":
			audioHandler(w, r)
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
