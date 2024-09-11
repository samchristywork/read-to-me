package main

import (
	texttospeech "cloud.google.com/go/texttospeech/apiv1"
	"cloud.google.com/go/texttospeech/apiv1/texttospeechpb"
	"context"
	"crypto/sha1"
	"fmt"
	"net/http"
	"os"
)

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

func tts(inputText string, shaSum string) error {
	textFilename := fmt.Sprintf("data/text-%s.txt", shaSum)
	outputFilename := fmt.Sprintf("data/output-%s.mp3", shaSum)

	if fileExists(outputFilename) {
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

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
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
