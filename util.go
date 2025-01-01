package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"golang.org/x/text/unicode/norm"
	"html/template"
	"log"
	"net/http"
)

func calculateHash(text string) string {
	hasher := sha256.New()
	normalizedText := norm.NFC.String(text)
	hasher.Write([]byte(normalizedText))
	return hex.EncodeToString(hasher.Sum(nil))
}

func httpError(w http.ResponseWriter, message string, e int) {
	page, err := renderPage(fmt.Sprintf("<main><h1>Error</h1><p>%s</p></main>", message))
	if err != nil {
		log.Printf("%s: %v", "Error rendering page", err)
		http.Error(w, "Internal Server Error: Unable to load page", e)
		return
	}

	_, err = fmt.Fprintln(w, page)
	if err != nil {
		log.Printf("%s: %v", "Error writing response", err)
	}
}

func form(action, content string) template.HTML {
	return template.HTML(fmt.Sprintf(`<form method="post" action="%s">%s</form>`,
		action, content))
}
