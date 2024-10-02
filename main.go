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
	"github.com/trietmn/go-wiki"
	"log"
	"math/rand"
	"net/http"
	"net/smtp"
	"regexp"
	"os"
	"strings"
	"io/ioutil"
	"time"
	"path/filepath"
)


type Session struct {
	Username string
	Expiry   int64
}

var sessionMap = make(map[string]Session)

func replaceIncludes(strContent string) (string, error) {
	re := regexp.MustCompile(`{{include\s+"([^"]+)"}}`)

	replacer := func(match string) string {
		groups := re.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}

		filename := groups[1]
		filePath := filepath.Join("static", filename)

		fileContent, err := ioutil.ReadFile(filePath)
		if err != nil {
			return ""
		}

		return string(fileContent)
	}

	modifiedContent := re.ReplaceAllStringFunc(strContent, replacer)
	return modifiedContent, nil
}

func middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(r.URL);
		next.ServeHTTP(w, r)
	})
}

func errorStatus(message string) string {
	return fmt.Sprintf("{\"status\": \"error\", \"error\": \"%s\"}", message)
}

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

	_, err = textFile.Write([]byte(inputText))
	if err != nil {
		return err
	}

	outFile, err := os.Create(outputFilename)
	if err != nil {
		return err
	}
	defer outFile.Close()

	_, err = outFile.Write(resp.AudioContent)
	if err != nil {
		return err
	}

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

func readJSONFile(filename string) (map[string]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	data := make(map[string]string)
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&data)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func sendRegistrationEmail(username string, email string, code string) error {
	creds, err := readJSONFile("creds.json")
	if err != nil {
		return err
	}

	to := []string{email}

	msg := []byte("To: " + email + "\r\n" +
		"Subject: Account Created\r\n" +
		"MIME-version: 1.0;\r\n" +
		"Content-Type: text/html; charset=\"UTF-8\";\r\n\r\n" +
		"\r\n" +
		"<html><body><h1>Registration</h1>" +
		"<a href='localhost:8080/verify.html?u=" + username + "&k=" + code + "'>Registration link</a>.\r\n" +
		username + "\r\n" +
		"</body></html>\r\n",
	)

	auth := smtp.PlainAuth(
		"",
		creds["sender"],
		creds["password"],
		"smtp.gmail.com",
	)

	err = smtp.SendMail(
		"smtp.gmail.com:587",
		auth,
		creds["sender"],
		to,
		msg,
	)
	if err != nil {
		return err
	}

	return nil
}

func main() {
	logName := "server.log"
	logFile, err := os.OpenFile(logName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		fmt.Println(err)
		return
	}

	log.SetOutput(logFile)

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

	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS users(
  ID INTEGER PRIMARY KEY,
  Joined_timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
  Username TEXT NOT NULL UNIQUE,
  PasswordHash TEXT NOT NULL,
  Email TEXT NOT NULL UNIQUE,
  Verified BOOLEAN DEFAULT FALSE,
  VerificationCode TEXT NOT NULL,
  Credits INTEGER DEFAULT 0
);`)
	if err != nil {
		fmt.Println(err)
		return
	}

	http.HandleFunc("/post", func(w http.ResponseWriter, r *http.Request) {
		var data struct {
			Sha   string `json:"sha"`
			Title string `json:"title"`
			Token string `json:"token"`
		}

		err := json.NewDecoder(r.Body).Decode(&data)
		if err != nil {
			http.Error(w, errorStatus("Bad Request"), http.StatusBadRequest)
			return
		}

		session, ok := sessionMap[data.Token]
		if !ok || session.Expiry < time.Now().Unix() {
			http.Error(w, errorStatus("Invalid Token"), http.StatusUnauthorized)
			return
		}

		_, err = db.Exec("INSERT INTO posts (title, sha1, username) VALUES (?, ?, ?)", data.Title, data.Sha, "sam")
		if err != nil {
			http.Error(w, errorStatus("Could Not Post"), http.StatusInternalServerError)
			return
		}

		_, err = fmt.Fprintf(w, "{\"status\": \"ok\"}")
		if err != nil {
			http.Error(w, errorStatus("Could Not Generate Reply"), http.StatusInternalServerError)
			return
		}
	})

	http.HandleFunc("/profile", func(w http.ResponseWriter, r *http.Request) {
		var data struct {
			Token string `json:"token"`
		}

		err := json.NewDecoder(r.Body).Decode(&data)
		if err != nil {
			http.Error(w, errorStatus("Bad Request"), http.StatusBadRequest)
			return
		}

		session, ok := sessionMap[data.Token]
		if !ok || session.Expiry < time.Now().Unix() {
			http.Error(w, errorStatus("Invalid Token"), http.StatusUnauthorized)
			return
		}

		var credits int
		err = db.QueryRow("SELECT Credits FROM users WHERE Username = ?", session.Username).Scan(&credits)
		if err != nil {
			http.Error(w, errorStatus("Could Not Get Profile"), http.StatusInternalServerError)
			return
		}

		response := struct {
			Status   string `json:"status"`
			Username string `json:"username"`
			Credits  int    `json:"credits"`
		}{
			Status:   "ok",
			Username: session.Username,
			Credits:  credits,
		}

		responseJSON, err := json.Marshal(response)
		if err != nil {
			http.Error(w, errorStatus("Could Not Get Profile"), http.StatusInternalServerError)
			return
		}

		_, err = w.Write(responseJSON)
		if err != nil {
			http.Error(w, errorStatus("Could Not Generate Reply"), http.StatusInternalServerError)
			return
		}
	})

	http.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		var data struct {
			Username string `json:"username"`
			Token    string `json:"token"`
		}

		err := json.NewDecoder(r.Body).Decode(&data)
		if err != nil {
			http.Error(w, errorStatus("Bad Request"), http.StatusBadRequest)
			return
		}

		session, ok := sessionMap[data.Token]
		if !ok || session.Expiry < time.Now().Unix() {
			http.Error(w, errorStatus("Invalid Token"), http.StatusUnauthorized)
			return
		}

		rows, err := db.Query("SELECT title FROM posts WHERE username = ?", data.Username)
		if err != nil {
			http.Error(w, errorStatus("Could Not Get Posts"), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var titles []string
		for rows.Next() {
			var title string
			err = rows.Scan(&title)
			if err != nil {
				http.Error(w, errorStatus("Could Not Get Posts"), http.StatusInternalServerError)
				return
			}

			titles = append(titles, title)
		}

		titlesJSON, err := json.Marshal(titles)
		if err != nil {
			http.Error(w, errorStatus("Could Not Get Posts"), http.StatusInternalServerError)
			return
		}

		_, err = fmt.Fprintf(w, string(titlesJSON))
		if err != nil {
			http.Error(w, errorStatus("Could Not Generate Reply"), http.StatusInternalServerError)
			return
		}
	})

	http.HandleFunc("/play", func(w http.ResponseWriter, r *http.Request) {
		var data struct {
			Session string `json:"session"`
			Token   string `json:"token"`
		}

		err := json.NewDecoder(r.Body).Decode(&data)
		if err != nil {
			http.Error(w, errorStatus("Bad Request"), http.StatusBadRequest)
			return
		}

		textFilename := fmt.Sprintf("data/session-%s.txt", data.Session)

		text, err := os.ReadFile(textFilename)
		if err != nil {
			http.Error(w, errorStatus("Bad Request"), http.StatusBadRequest)
		}

		fragments := splitText(string(text))

		for i := 0; i < len(fragments); i++ {
			if len(fragments[i]) == 0 {
				fragments = append(fragments[:i], fragments[i+1:]...)
				i--
			}
		}

		shas := make([]string, len(fragments))
		shaChan := make(chan string)

		for n, fragment := range fragments {
			go func(fragment string, n int) {
				sha1 := fmt.Sprintf("%x", sha1.Sum([]byte(fragment)))
				shas[n] = fmt.Sprintf("\"%s\"", sha1)
				shaChan <- sha1
			}(fragment, n)
		}

		for i := 0; i < len(fragments); i++ {
			select {
			case sha := <-shaChan:
				fmt.Println("Processed", sha)
			}
		}

		_, err = fmt.Fprintf(w, "{"+
			"\"status\": \"ok\","+
			"\"shas\": ["+strings.Join(shas, ", ")+"]}")
		if err != nil {
			http.Error(w, errorStatus("Could Not Generate Reply"), http.StatusInternalServerError)
			return
		}
	})

	http.HandleFunc("/synthesize", func(w http.ResponseWriter, r *http.Request) {
		var data struct {
			Text  string `json:"text"`
			Token string `json:"token"`
		}

		err := json.NewDecoder(r.Body).Decode(&data)
		if err != nil {
			http.Error(w, errorStatus("Bad Request"), http.StatusBadRequest)
			return
		}

		sessionID := fmt.Sprintf("%x", sha1.Sum([]byte(data.Text)))
		sessionFilename := fmt.Sprintf("data/session-%s.txt", sessionID)

		sessionFile, err := os.Create(sessionFilename)
		if err != nil {
			http.Error(w, errorStatus("Could Not Synthesize Audio"), http.StatusInternalServerError)
			return
		}
		defer sessionFile.Close()

		_, err = sessionFile.Write([]byte(data.Text))
		if err != nil {
			http.Error(w, errorStatus("Could Not Synthesize Audio"), http.StatusInternalServerError)
			return
		}

		fragments := splitText(data.Text)

		for i := 0; i < len(fragments); i++ {
			if len(fragments[i]) == 0 {
				fragments = append(fragments[:i], fragments[i+1:]...)
				i--
			}
		}

		err, shas := processFragments(fragments)
		if err != nil {
			http.Error(w, errorStatus("Could Not Synthesize Audio"), http.StatusInternalServerError)
		}

		_, err = fmt.Fprintf(w, "{"+
			"\"status\": \"ok\","+
			"\"shas\": ["+strings.Join(shas, ", ")+"],"+
			"\"sessionID\": \""+sessionID+"\"}")
		if err != nil {
			http.Error(w, errorStatus("Could Not Generate Reply"), http.StatusInternalServerError)
			return
		}
	})

	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		var data struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}

		err := json.NewDecoder(r.Body).Decode(&data)
		if err != nil {
			http.Error(w, errorStatus("Bad Request"), http.StatusBadRequest)
			return
		}

		var passwordHash string
		err = db.QueryRow("SELECT PasswordHash FROM users WHERE username = ?", data.Username).Scan(&passwordHash)
		if err != nil {
			http.Error(w, errorStatus("Login Failed"), http.StatusUnauthorized)
			return
		}

		hash := fmt.Sprintf("%x", sha1.Sum([]byte(data.Password)))
		if hash != passwordHash {
			http.Error(w, errorStatus("Login Failed"), http.StatusUnauthorized)
			return
		}

		response := struct {
			Status   string `json:"status"`
			Username string `json:"username"`
			Token    string `json:"token"`
		}{
			Status:   "ok",
			Username: data.Username,
			Token:    fmt.Sprintf("%x", sha1.Sum([]byte(fmt.Sprintf("%d", rand.Int63())))),
		}

		sessionMap[response.Token] = Session{
			Username: data.Username,
			Expiry:   time.Now().Unix() + 3600,
		}

		responseJSON, err := json.Marshal(response)
		if err != nil {
			http.Error(w, errorStatus("Login Failed"), http.StatusInternalServerError)
			return
		}

		_, err = fmt.Fprintf(w, string(responseJSON))
		if err != nil {
			http.Error(w, errorStatus("Could Not Generate Reply"), http.StatusInternalServerError)
			return
		}
	})

	http.HandleFunc("/signup", func(w http.ResponseWriter, r *http.Request) {
		var data struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Email    string `json:"email"`
		}

		err := json.NewDecoder(r.Body).Decode(&data)
		if err != nil {
			http.Error(w, errorStatus("Bad Request"), http.StatusBadRequest)
			return
		}

		hash := fmt.Sprintf("%x", sha1.Sum([]byte(data.Password)))

		code := fmt.Sprintf("%x", sha1.Sum([]byte(fmt.Sprintf("%d", rand.Int63()))))

		_, err = db.Exec(`INSERT INTO users
		(Username, Email, PasswordHash, VerificationCode)
		VALUES (?, ?, ?, ?)`, data.Username, data.Email, hash, code)
		if err != nil {
			log.Println(err)
			http.Error(w, errorStatus("Could Not Register User"), http.StatusInternalServerError)
			return
		}

		err = sendRegistrationEmail(data.Username, data.Email, code)
		if err != nil {
			log.Println(err)
			http.Error(w, errorStatus("Could Not Register User"), http.StatusInternalServerError)
			return
		}

		_, err = fmt.Fprintf(w, "{\"status\": \"ok\"}")
		if err != nil {
			http.Error(w, errorStatus("Could Not Generate Reply"), http.StatusInternalServerError)
			return
		}
	})

	http.HandleFunc("/wikipedia", func(w http.ResponseWriter, r *http.Request) {
		var data struct {
			Title string `json:"title"`
			Token string `json:"token"`
		}

		err := json.NewDecoder(r.Body).Decode(&data)
		if err != nil {
			http.Error(w, errorStatus("Bad Request"), http.StatusBadRequest)
			return
		}

		session, ok := sessionMap[data.Token]
		if !ok || session.Expiry < time.Now().Unix() {
			http.Error(w, errorStatus("Invalid Token"), http.StatusUnauthorized)
			return
		}

		title := data.Title

		fmt.Println("Searching for", title)

		searchResult, _, err := gowiki.Search(title, 1, false)
		if err != nil {
			http.Error(w, errorStatus("Search Failed"), http.StatusInternalServerError)
			return
		}

		if len(searchResult) == 0 {
			http.Error(w, errorStatus("Search Failed"), http.StatusInternalServerError)
		}

		page, err := gowiki.GetPage(searchResult[0], -1, false, true)
		if err != nil {
			http.Error(w, errorStatus("Search Failed"), http.StatusInternalServerError)
			return
		}

		title = page.Title
		url := page.URL
		content, err := page.GetContent()
		if err != nil {
			http.Error(w, errorStatus("Search Failed"), http.StatusInternalServerError)
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
			http.Error(w, errorStatus("Search Failed"), http.StatusInternalServerError)
			return
		}

		_, err = w.Write(responseJSON)
		if err != nil {
			http.Error(w, errorStatus("Could Not Generate Reply"), http.StatusInternalServerError)
			return
		}
	})

	http.HandleFunc("/verify", func(w http.ResponseWriter, r *http.Request) {
		var data struct {
			Key      string `json:"key"`
			Username string `json:"username"`
		}

		err := json.NewDecoder(r.Body).Decode(&data)
		if err != nil {
			http.Error(w, errorStatus("Bad Request"), http.StatusBadRequest)
			return
		}

		username := data.Username
		key := data.Key

		var code string
		err = db.QueryRow("SELECT VerificationCode FROM users WHERE Username = ?", username).Scan(&code)
		if err != nil {
			http.Error(w, errorStatus("Could Not Verify User"), http.StatusInternalServerError)
			return
		}

		if key != code {
			http.Error(w, errorStatus("Could Not Verify User"), http.StatusUnauthorized)
			return
		}

		_, err = db.Exec("UPDATE users SET Verified = TRUE WHERE Username = ?", username)
		if err != nil {
			http.Error(w, errorStatus("Could Not Verify User"), http.StatusInternalServerError)
			return
		}

		_, err = db.Exec("UPDATE users SET Credits = 10000 WHERE Username = ?", username)
		if err != nil {
			http.Error(w, errorStatus("Could Not Verify User"), http.StatusInternalServerError)
			return
		}

		_, err = fmt.Fprintf(w, "{\"status\": \"ok\"}")
		if err != nil {
			http.Error(w, errorStatus("Could Not Generate Reply"), http.StatusInternalServerError)
			return
		}
	})

	http.Handle("/data/", http.StripPrefix("/data/", http.FileServer(http.Dir("data"))))

	http.Handle("/", http.FileServer(http.Dir("static")))

	fmt.Println("Listening on port 8080")
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal(err)
	}
}
