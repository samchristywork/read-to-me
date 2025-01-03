package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"log"
	"time"

	texttospeech "cloud.google.com/go/texttospeech/apiv1"
	"cloud.google.com/go/texttospeech/apiv1/texttospeechpb"
	_ "github.com/mattn/go-sqlite3"
	"github.com/tcolgate/mp3"
	"golang.org/x/text/unicode/norm"
)

func calculateHash(text string) string {
	hasher := sha256.New()
	normalizedText := norm.NFC.String(text)
	hasher.Write([]byte(normalizedText))
	return hex.EncodeToString(hasher.Sum(nil))
}

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
		log.Printf("%s: %v", "Error checking existing audio", err)
		return "", "", nil, 0, err
	}

	if audioContent != nil {
		log.Printf("Audio for text already exists, using cached version.")
		return audioHash, text, audioContent, audioLength, nil
	}

	ctx := context.Background()
	client, err := texttospeech.NewClient(ctx)
	if err != nil {
		log.Printf("%s: %v", "Error creating text-to-speech client", err)
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
		log.Printf("%s: %v", "Error synthesizing text", err)
		_, t, c, l, _ := generateTTS("Text could not be synthesized.")
		_, err = db.Exec(`INSERT INTO audio (hash, text, audio, audio_length_ms)
			VALUES (?, ?, ?, ?)`, audioHash, t, c, l)
		return "", "", nil, 0, err
	}

	audioContent = resp.AudioContent
	audioLength, err = calculateAudioLength(audioContent)
	if err != nil {
		log.Printf("%s: %v", "Error calculating audio length", err)
		return "", "", nil, 0, err
	}

	dbMutex.Lock()
	_, err = db.Exec(`INSERT INTO audio (hash, text, audio, audio_length_ms)
	VALUES (?, ?, ?, ?)`, audioHash, text, audioContent, audioLength)
	dbMutex.Unlock()

	if err != nil {
		log.Printf("%s: %v", "Error inserting audio into database", err)
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
		log.Printf("%s: %v", "Error fetching audio cache", err)
		return nil, 0, err
	}

	return audioContent, audioLength, nil
}
