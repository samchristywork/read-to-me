package main

import (
	"fmt"
	"html/template"
	"strings"
)

func post(id, title, url, content, author, time, class string) template.HTML {
	return template.HTML(fmt.Sprintf(`
		<div class="post %s">
			<h3><a href="/post?id=%s">%s</a> - <a href="%s">%s</a></h3>
			<em data-timestamp="%s">%s at %s</em>
			<div class="post-body">%s</div>
		</div>
	`, class, id, title, url, "source", time, author, time, content))
}

func formatTime(ms int) string {
	seconds := ms / 1000
	minutes := seconds / 60
	hours := minutes / 60

	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes%60, seconds%60)
	} else if minutes > 0 {
		return fmt.Sprintf("%d:%02d", minutes, seconds%60)
	} else {
		return fmt.Sprintf("%d", seconds)
	}
}

func createPost(title, source, body string) string {
	title = strings.ReplaceAll(title, `"`, "")
	source = strings.ReplaceAll(source, `"`, "")
	body = strings.ReplaceAll(body, `"`, "")

	var collectionDropdown string
	rows, err := db.Query(`SELECT id, title FROM collections WHERE author = "Anonymous"`)
	if err != nil {
		perror("Error querying collections", err)
	} else {
		defer rows.Close()

		collectionDropdown = `<label for="collection">Collection:</label>
		<select id="collection" name="collection">
			<option value="">None</option>`
		for rows.Next() {
			var id int
			var title string
			err := rows.Scan(&id, &title)
			if err != nil {
				perror("Error scanning row", err)
				continue
			}
			collectionDropdown += fmt.Sprintf(`<option value="%d">%s</option>`, id, title)
		}
		collectionDropdown += `</select><br>`
	}

	return fmt.Sprintf(`
		<main>
			%s
			<div class="datasources">
				<div class="datasource">%s</div>
				<div class="datasource">%s</div>
				<div class="datasource">%s</div>
			</div>
		</main>`,
		string(form("new-post", `
			<label for="title">Title:</label>
			<input type="text" id="title" name="title" value="`+title+`" autofocus required><br>
			`+collectionDropdown+`
			<label for="source">Source:</label>
			<input type="url" id="source" name="source" placeholder="https://example.com" value="`+source+`"><br>
			<label for="content">Body:</label>
			<textarea id="content" name="content" cols="80" rows="15" required>`+body+`</textarea><br>
			<input type="submit" value="Submit"/>
		`)),
		string(form("create-post", `
			<label for="keyword">Wikipedia:</label>
			<input type="text" id="keyword" name="keyword" required><br>
			<input type="hidden" id="source" name="source" value="Wikipedia">
			<input type="submit" value="Search"/>
		`)),
		string(form("create-post", `
			<label for="keyword">Wikiquote:</label>
			<input type="text" id="keyword" name="keyword" required><br>
			<input type="hidden" id="source" name="source" value="Wikiquote">
			<input type="submit" value="Search"/>
		`)),
		string(form("create-post", `
			<label for="keyword">Link:</label>
			<input type="text" id="keyword" name="keyword" required><br>
			<input type="hidden" id="source" name="source" value="Link">
			<input type="submit" value="Search"/>
		`)),
	)
}
