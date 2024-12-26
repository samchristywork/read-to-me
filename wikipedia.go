package main

import (
	"fmt"
	"github.com/trietmn/go-wiki"
	"strings"
	"net/url"
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
