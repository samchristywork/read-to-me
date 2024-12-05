package main

import (
	"fmt"
	"github.com/k3a/html2text"
	"io/ioutil"
	"net/http"
)

func fetchLinkContent(url string) (string, string, string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to fetch link: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("failed to fetch link: received status code %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read link content: %v", err)
	}

	text := html2text.HTML2Text(string(body))

	return "", url, text, nil
}
