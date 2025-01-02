package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"golang.org/x/text/unicode/norm"
	"html/template"
)

func calculateHash(text string) string {
	hasher := sha256.New()
	normalizedText := norm.NFC.String(text)
	hasher.Write([]byte(normalizedText))
	return hex.EncodeToString(hasher.Sum(nil))
}

func form(action, content string) template.HTML {
	return template.HTML(fmt.Sprintf(`<form method="post" action="%s">%s</form>`,
		action, content))
}
