package main

import (
	"net/http"
	"fmt"
	"log"
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		default:
			fs := http.FileServer(http.Dir("./static"))
			fs.ServeHTTP(w, r)
		}
	})

	fmt.Println("Starting server on :4343")
	if err := http.ListenAndServe(":4343", mux); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}
