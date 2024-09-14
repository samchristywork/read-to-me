package main

import (
	texttospeech "cloud.google.com/go/texttospeech/apiv1"
	"cloud.google.com/go/texttospeech/apiv1/texttospeechpb"
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"log"
	"net/http"
	"os"
	"strings"
	"github.com/trietmn/go-wiki"
)

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

func tts(inputText string, shaSum string) error {
	textFilename := fmt.Sprintf("data/text-%s.txt", shaSum)
	outputFilename := fmt.Sprintf("data/output-%s.mp3", shaSum)

	if fileExists(outputFilename) {
		fmt.Println("File already exists:", outputFilename)
		return nil
	}

	ctx := context.Background()

	client, err := texttospeech.NewClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	req := texttospeechpb.SynthesizeSpeechRequest{
		Input: &texttospeechpb.SynthesisInput{
			InputSource: &texttospeechpb.SynthesisInput_Text{Text: string(inputText)},
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
		return err
	}

	textFile, err := os.Create(textFilename)
	if err != nil {
		return err
	}
	defer textFile.Close()
	textFile.Write([]byte(inputText))

	outFile, err := os.Create(outputFilename)
	if err != nil {
		return err
	}
	defer outFile.Close()
	outFile.Write(resp.AudioContent)

	return nil
}

func splitText(text string) []string {
	return strings.Split(text, "\n")
}

func processFragments(fragments []string) (error, []string) {
	shas := make([]string, len(fragments))
	errChan := make(chan error)
	shaChan := make(chan string)

	for n, fragment := range fragments {
		go func(fragment string, n int) {
			sha1 := fmt.Sprintf("%x", sha1.Sum([]byte(fragment)))
			shas[n] = fmt.Sprintf("\"%s\"", sha1)

			err := tts(fragment, sha1)
			if err != nil {
				errChan <- err
				return
			}

			shaChan <- sha1
		}(fragment, n)
	}

	for i := 0; i < len(fragments); i++ {
		select {
		case err := <-errChan:
			return err, shas
		case sha := <-shaChan:
			fmt.Println("Processed", sha)
		}
	}

	return nil, shas
}

func main() {
	filename := "data.sqlite"
	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer db.Close()

	_, err = db.Exec("CREATE TABLE IF NOT EXISTS posts (title TEXT, sha1 TEXT, username TEXT, timestamp DATETIME DEFAULT CURRENT_TIMESTAMP)")
	if err != nil {
		fmt.Println(err)
		return
	}

	_, err = db.Exec("CREATE TABLE IF NOT EXISTS users (username TEXT, email TEXT, passwordHash TEXT)")
	if err != nil {
		fmt.Println(err)
		return
	}

	http.HandleFunc("/create.html", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})

	http.HandleFunc("/user.html", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "user.html")
	})

	http.HandleFunc("/login.html", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "login.html")
	})

	http.HandleFunc("/signup.html", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "signup.html")
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})

	http.HandleFunc("/post", func(w http.ResponseWriter, r *http.Request) {
		var data struct {
			Sha   string `json:"sha"`
			Title string `json:"title"`
		}

		err := json.NewDecoder(r.Body).Decode(&data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		_, err = db.Exec("INSERT INTO posts (title, sha1) VALUES (?, ?)", data.Title, data.Sha)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		fmt.Fprintf(w, "{\"status\": \"ok\"}")
	})

	http.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		var data struct {
			Username string `json:"username"`
		}

		err := json.NewDecoder(r.Body).Decode(&data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		fmt.Println("Received user:", data.Username)

		rows, err := db.Query("SELECT title FROM posts WHERE username = ?", data.Username)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var titles []string
		for rows.Next() {
			var title string
			err = rows.Scan(&title)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			titles = append(titles, title)
		}

		titlesJSON, err := json.Marshal(titles)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		fmt.Fprintf(w, string(titlesJSON))
	})

	http.HandleFunc("/synthesize", func(w http.ResponseWriter, r *http.Request) {
		var data struct {
			Text string `json:"text"`
		}

		err := json.NewDecoder(r.Body).Decode(&data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		sessionID := fmt.Sprintf("%x", sha1.Sum([]byte(data.Text)))
		sessionFilename := fmt.Sprintf("data/session-%s.txt", sessionID)

		sessionFile, err := os.Create(sessionFilename)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer sessionFile.Close()
		sessionFile.Write([]byte(data.Text))

		fragments := splitText(data.Text)

		for i := 0; i < len(fragments); i++ {
			if len(fragments[i]) == 0 {
				fragments = append(fragments[:i], fragments[i+1:]...)
				i--
			}
		}

		err, shas := processFragments(fragments)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		fmt.Fprintf(w, "{"+
			"\"shas\": ["+strings.Join(shas, ", ")+"],"+
			"\"sessionID\": \""+sessionID+"\"}")
	})

	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		username := r.Form.Get("username")
		password := r.Form.Get("password")

		var passwordHash string
		err := db.QueryRow("SELECT passwordHash FROM users WHERE username = ?", username).Scan(&passwordHash)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		hash := fmt.Sprintf("%x", sha1.Sum([]byte(password)))
		if hash != passwordHash {
			http.Error(w, "Invalid password", http.StatusUnauthorized)
			return
		}

		fmt.Fprintf(w, "{\"status\": \"ok\"}")
	})

	http.HandleFunc("/signup", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		username := r.Form.Get("username")
		email := r.Form.Get("email")
		password := r.Form.Get("password")

		hash := fmt.Sprintf("%x", sha1.Sum([]byte(password)))

		_, err := db.Exec("INSERT INTO users (username, email, passwordHash) VALUES (?, ?, ?)", username, email, hash)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		fmt.Fprintf(w, "{\"status\": \"ok\"}")
	})

	http.HandleFunc("/wikipedia", func(w http.ResponseWriter, r *http.Request) {
		var data struct {
			Title   string `json:"title"`
		}

		err := json.NewDecoder(r.Body).Decode(&data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		title := data.Title

		fmt.Println("Searching for", title)

		searchResult, _, err := gowiki.Search(title, 1, false)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if len(searchResult) == 0 {
			http.Error(w, "No results found", http.StatusNotFound)
			return
		}

		page, err := gowiki.GetPage(searchResult[0], -1, false, true)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		title = page.Title
		url := page.URL
		content, err := page.GetContent()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		response := struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		}{
			Title:   title,
			URL:     url,
			Content: content,
		}

		responseJSON, err := json.Marshal(response)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		fmt.Fprintf(w, string(responseJSON))
	})

	http.Handle("/data/", http.StripPrefix("/data/", http.FileServer(http.Dir("data"))))

	fmt.Println("Listening on port 8080")
	http.ListenAndServe(":8080", nil)
}
