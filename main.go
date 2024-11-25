package main

import (
	"net/http"
	"fmt"
	"log"
	"database/sql"
	"os"
	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

func main() {
	var err error
	db, err = sql.Open("sqlite3", "./data.db")
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		default:
			fs := http.FileServer(http.Dir("./static"))
			fs.ServeHTTP(w, r)
		}
	})

	fmt.Println("Starting server on :4343")
	if err = http.ListenAndServe(":4343", mux); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}
