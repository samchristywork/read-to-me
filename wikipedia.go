package main

import (
	"fmt"
	"github.com/trietmn/go-wiki"
	"strings"
)

func fetchWikipediaContent(keyword string) (string, string, string, error) {
	searchResult, _, err := gowiki.Search(keyword, 1, false)
	if err != nil || len(searchResult) == 0 {
		return "", "", "", fmt.Errorf("Error fetching Wikipedia article")
	}

	p, err := gowiki.GetPage(searchResult[0], -1, false, true)
	if err != nil {
		return "", "", "", err
	}

	content, err := p.GetContent()
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
	return p.Title, p.URL, contentBuilder.String(), nil
}
