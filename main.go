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

func perror(message string, err error) {
	log.Printf("%s: %v", message, err)
}

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
		perror("Error checking existing audio", err)
		return "", "", nil, 0, err
	}

	if audioContent != nil {
		log.Printf("Audio for text already exists, using cached version.")
		return audioHash, text, audioContent, audioLength, nil
	}

	ctx := context.Background()
	client, err := texttospeech.NewClient(ctx)
	if err != nil {
		perror("Error creating text-to-speech client", err)
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

	resp, err := client.SynthesizeSpeech(ctx, &req)
	if err != nil {
		perror("Error synthesizing text", err)
		_, t, c, l, _ := generateTTS("Text could not be synthesized.")
		_, err = db.Exec(`INSERT INTO audio (hash, text, audio, audio_length_ms)
			VALUES (?, ?, ?, ?)`, audioHash, t, c, l)
		return "", "", nil, 0, err
	}

	audioContent = resp.AudioContent
	audioLength, err = calculateAudioLength(audioContent)
	if err != nil {
		perror("Error calculating audio length", err)
		return "", "", nil, 0, err
	}

	dbMutex.Lock()
	_, err = db.Exec(`INSERT INTO audio (hash, text, audio, audio_length_ms)
	VALUES (?, ?, ?, ?)`, audioHash, text, audioContent, audioLength)
	dbMutex.Unlock()

	if err != nil {
		perror("Error inserting audio into database", err)
		return "", "", nil, 0, err
	}

	return audioHash, text, audioContent, audioLength, nil
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
		perror("Error fetching audio cache", err)
		return nil, 0, err
	}

	return audioContent, audioLength, nil
}

func renderPage(content string) (string, error) {
	tmpl, err := templateFiles.ReadFile("template/head.html")
	if err != nil {
		perror("Error reading head template", err)
		return "", err
	}
	head := string(tmpl)

	tmpl, err = templateFiles.ReadFile("template/nav.html")
	if err != nil {
		perror("Error reading nav template", err)
		return "", err
	}
	nav := string(tmpl)

	tmpl, err = templateFiles.ReadFile("template/footer.html")
	if err != nil {
		perror("Error reading footer template", err)
		return "", err
	}
	footer := string(tmpl)

	script := `<script>
		document.querySelectorAll(".post em").forEach((e) => {
			const timestamp = e.getAttribute("data-timestamp");
			const localDate = new Date(timestamp).toLocaleString(undefined, {
				timeZoneName: "short"
			});
			e.textContent = e.textContent.split(" at ")[0] + " at " + localDate;
		});
	</script>`

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>%s</head>
<body>%s%s%s</body>%s
</html>`, head, nav, content, footer, script), nil
}

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	page, err := renderPage("<main><h1>Error - Page not found</h1></main>")
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

func form(action, content string) template.HTML {
	return template.HTML(fmt.Sprintf(`<form method="post" action="%s">%s</form>`,
		action, content))
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

func audioHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		perror("Post ID is required", nil)
		http.Error(w, "Post ID is required", http.StatusBadRequest)
		return
	}

	stmt, err := db.Prepare("SELECT content FROM posts WHERE id = ?")
	if err != nil {
		perror("Error preparing SQL statement", err)
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
		perror("Error querying post", err)
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
			perror("Error retrieving audio cache", err)
			http.Error(w, "Internal Server Error: Unable to retrieve audio",
				http.StatusInternalServerError)
			return
		}

		audioBytes = append(audioBytes, audioContent)
	}

	fullAudio := bytes.Join(audioBytes, []byte(""))

	http.ServeContent(w, r, "audio.mp3", time.Now(), bytes.NewReader(fullAudio))
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
		perror("Error creating posts table", err)
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
		perror("Error creating audio table", err)
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
		perror("Error creating collections table", err)
		return
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS collection_posts (
		collection_id INTEGER,
		post_id INTEGER
	)`)
	if err != nil {
		perror("Error creating collection_posts table", err)
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
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				notFoundHandler(w, r)
			} else {
				fs.ServeHTTP(w, r)
			}
		}
	})

	fmt.Println("Starting server on :4343")
	err = http.ListenAndServe(":4343", mux)
	if err != nil {
		perror("Error starting server", err)
	}
}
