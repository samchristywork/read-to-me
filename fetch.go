package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/k3a/html2text"
	"github.com/trietmn/go-wiki"
)

func fetchWikipediaContent(keyword string) (string, string, string, error) {
	keyword = url.QueryEscape(keyword)

	searchResult, _, err := gowiki.Search(keyword, 1, false)
	if err != nil || searchResult == nil || len(searchResult) == 0 {
		return "", "", "", fmt.Errorf("Could not find any wikipedia page for keyword: %s", keyword)
	}

	page, err := gowiki.GetPage(searchResult[0], -1, false, true)
	if err != nil {
		return "", "", "", err
	}

	content, err := page.GetContent()
	if err != nil {
		return "", "", "", err
	}

	var contentBuilder strings.Builder
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.Trim(line, "=")
		if trimmed != line {
			contentBuilder.WriteString("Section" + trimmed + "\n")
		} else {
			contentBuilder.WriteString(trimmed + "\n")
		}
	}
	return page.Title, page.URL, contentBuilder.String(), nil
}

func fetchWikiquoteContent(keyword string) (string, string, string, error) {
	keyword = url.QueryEscape(keyword)
	baseURL := "https://en.wikiquote.org/w/api.php"
	query := fmt.Sprintf("?format=json&action=query&list=search&srsearch=%s", keyword)
	fullURL := baseURL + query

	resp, err := http.Get(fullURL)
	if err != nil {
		return "", "", "", fmt.Errorf("Could not connect to Wikiquote: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", "", "", fmt.Errorf("Could not read response body: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", "", fmt.Errorf("Could not parse JSON: %v", err)
	}

	searchList, found := result["query"].(map[string]interface{})["search"].([]interface{})
	if !found || len(searchList) == 0 {
		return "", "", "", fmt.Errorf("No results for keyword: %s", keyword)
	}

	firstResult := searchList[0].(map[string]interface{})
	rawTitle := firstResult["title"].(string)

	title := url.QueryEscape(rawTitle)

	pageContentURL := fmt.Sprintf("%s?format=json&action=query&prop=extracts&titles=%s", baseURL, title)
	resp, err = http.Get(pageContentURL)
	if err != nil {
		return "", "", "", fmt.Errorf("Could not connect to Wikiquote for page content: %v", err)
	}
	defer resp.Body.Close()

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", "", "", fmt.Errorf("Could not read page content response: %v", err)
	}

	if err = json.Unmarshal(body, &result); err != nil {
		return "", "", "", fmt.Errorf("Could not parse page content JSON: %v", err)
	}

	pages := result["query"].(map[string]interface{})["pages"].(map[string]interface{})
	var content string
	for _, page := range pages {
		pageMap := page.(map[string]interface{})
		content = pageMap["extract"].(string)
		break
	}

	content = html2text.HTML2Text(content)

	return rawTitle, fmt.Sprintf("https://en.wikiquote.org/wiki/%s", strings.ReplaceAll(rawTitle, " ", "_")), content, nil
}

func fetchLinkContent(url string) (string, string, string, error) {
	client := http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to fetch link: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("failed to fetch link: received status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read link content: %v", err)
	}

	text := html2text.HTML2Text(string(body))
	return "", url, text, nil
}
