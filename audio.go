package main

import (
	"bytes"
	"context"
	"database/sql"
	"log"
	"net/http"
	"strings"
	"time"

	texttospeech "cloud.google.com/go/texttospeech/apiv1"
	"cloud.google.com/go/texttospeech/apiv1/texttospeechpb"
	_ "github.com/mattn/go-sqlite3"
	"github.com/tcolgate/mp3"
)

func calculateAudioLength(audioContent []byte) (int, error) {
	reader := bytes.NewReader(audioContent)
	decoder := mp3.NewDecoder(reader)

	var frame mp3.Frame
	var skipped int
	var totalDuration time.Duration

	for {
		err := decoder.Decode(&frame, &skipped)
		if err != nil {
			break
		}
		totalDuration += frame.Duration()
	}

	return int(totalDuration.Milliseconds()), nil
}

func generateTTS(text string) (string, string, []byte, int, error) {
	audioHash := calculateHash(text)

	audioContent, audioLength, err := retrieveTTS(text)
	if err != nil && err != sql.ErrNoRows {
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

	if err == sql.ErrNoRows {
		return nil, 0, err
	}

	if err != nil {
		perror("Error fetching audio cache", err)
		return nil, 0, err
	}

	return audioContent, audioLength, nil
}

func audioHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		badRequest(w, "Post ID is required")
		return
	}

	stmt, err := db.Prepare("SELECT content FROM posts WHERE id = ?")
	if err != nil {
		internalServerError(w, "Unable to retrieve post")
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
		internalServerError(w, "Unable to retrieve post")
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
			internalServerError(w, "Unable to retrieve audio")
			return
		}

		audioBytes = append(audioBytes, audioContent)
	}

	fullAudio := bytes.Join(audioBytes, []byte(""))

	http.ServeContent(w, r, "audio.mp3", time.Now(), bytes.NewReader(fullAudio))
}
