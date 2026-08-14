package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func main() {
	endpoints := []struct {
		method string
		url    string
		body   string
	}{
		{"GET", "http://localhost:8080/health", ""},
		{"GET", "http://localhost:8080/api/search?q=oil+filter", ""},
		{"GET", "http://localhost:8080/api/oem/26300-35503", ""},
		{"GET", "http://localhost:8080/api/part/1/vehicles", ""},
		{"GET", "http://localhost:8080/api/part/1/chain", ""},
	}

	client := &http.Client{Timeout: 5 * time.Second}

	for _, ep := range endpoints {
		req, _ := http.NewRequest(ep.method, ep.url, nil)
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("FAIL %s %s → %v\n", ep.method, ep.url, err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		status := "OK"
		if resp.StatusCode >= 400 {
			status = "ERR"
		}
		// Truncate body for display
		b := string(body)
		if len(b) > 200 {
			b = b[:200] + "..."
		}
		fmt.Printf("%s %s %s [%d] → %s\n", status, ep.method, ep.url, resp.StatusCode, b)
	}

	os.Exit(0)
}
