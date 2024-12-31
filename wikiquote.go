package main

import (
	"encoding/json"
	"fmt"
	"github.com/k3a/html2text"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

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
