package main

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

func main() {
	client := &http.Client{Timeout: 10 * time.Second}

	tests := []string{
		"http://localhost:8080/api/search?q=OC+205&limit=5",
		"http://localhost:8080/api/search?q=W+811/80&limit=5",
		"http://localhost:8080/health",
	}

	for _, u := range tests {
		fmt.Printf("GET %s\n", u)
		resp, err := client.Get(u)
		if err != nil {
			fmt.Printf("  ERROR: %v\n", err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		fmt.Printf("  %d: %s\n\n", resp.StatusCode, string(body[:min(len(body), 200)]))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
