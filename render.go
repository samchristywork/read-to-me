package main

import (
	"fmt"
	"html/template"
	"log"
	"strings"
)

func form(action, content string) template.HTML {
	return template.HTML(fmt.Sprintf(`<form method="post" action="%s">%s</form>`,
		action, content))
}

func renderPage(content string) (string, error) {
	tmpl, err := templateFiles.ReadFile("template/head.html")
	if err != nil {
		log.Printf("%s: %v", "Error reading head template", err)
		return "", err
	}
	head := string(tmpl)

	tmpl, err = templateFiles.ReadFile("template/nav.html")
	if err != nil {
		log.Printf("%s: %v", "Error reading nav template", err)
		return "", err
	}
	nav := string(tmpl)

	tmpl, err = templateFiles.ReadFile("template/footer.html")
	if err != nil {
		log.Printf("%s: %v", "Error reading footer template", err)
		return "", err
	}
	footer := string(tmpl)

	script := `<script>
		document.querySelectorAll(".post em").forEach((e) => {
			const timestamp = e.getAttribute("data-timestamp");
			const localDate = new Date(timestamp).toLocaleString(undefined, {
				timeZoneName: "short"
			});
			e.textContent = e.textContent.split(" at ")[0] + " at " + localDate;
		});
	</script>`

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>%s</head>
<body>%s%s%s</body>%s
</html>`, head, nav, content, footer, script), nil
}

func createPost(title, source, body string) string {
	title = strings.ReplaceAll(title, `"`, "")
	source = strings.ReplaceAll(source, `"`, "")
	body = strings.ReplaceAll(body, `"`, "")

	var collectionDropdown string
	rows, err := db.Query(`SELECT id, title FROM collections WHERE author = "Anonymous"`)
	if err != nil {
		log.Printf("%s: %v", "Error querying collections", err)
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
				log.Printf("%s: %v", "Error scanning row", err)
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
