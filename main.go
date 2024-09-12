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

func main() {
	filename := "data.sqlite"
	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer db.Close()

	_, err = db.Exec("CREATE TABLE IF NOT EXISTS posts (title TEXT, sha1 TEXT)")

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

	http.HandleFunc("/synthesize", func(w http.ResponseWriter, r *http.Request) {
		text := r.FormValue("text")
		sha1 := fmt.Sprintf("%x", sha1.Sum([]byte(text)))
		fmt.Println("Text:", text)
		fmt.Println("SHA1:", sha1)

		http.ServeFile(w, r, fmt.Sprintf("data/output-%s.mp3", sha1))
	})

	fmt.Println("Listening on port 8080")
	http.ListenAndServe(":8080", nil)
}
