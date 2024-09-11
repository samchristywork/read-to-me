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
