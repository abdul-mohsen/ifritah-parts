package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type SearchResponse struct {
	Query          string `json:"query"`
	Total          int    `json:"total"`
	SearchStrategy string `json:"searchStrategy"`
	Results        []struct {
		Description string  `json:"description"`
		BrandName   string  `json:"brandName"`
		Confidence  float64 `json:"confidence"`
	} `json:"results"`
}

func main() {
	f, err := os.Open("not_found_parts.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	var parts []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		p := strings.TrimSpace(scanner.Text())
		if p != "" {
			parts = append(parts, p)
		}
	}

	fmt.Printf("Testing %d previously NOT FOUND parts...\n\n", len(parts))

	client := &http.Client{Timeout: 30 * time.Second}
	found := 0
	notFound := 0
	errors := 0
	strategyCount := make(map[string]int)
	var notFoundList []string

	for i, p := range parts {
		u := fmt.Sprintf("http://localhost:8080/api/search?q=%s", url.QueryEscape(p))
		resp, err := client.Get(u)
		if err != nil {
			errors++
			fmt.Printf("  [%d/%d] ERROR %s: %v\n", i+1, len(parts), p, err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var sr SearchResponse
		json.Unmarshal(body, &sr)

		if sr.Total > 0 {
			found++
			strategyCount[sr.SearchStrategy]++
			desc := ""
			if len(sr.Results) > 0 {
				desc = sr.Results[0].Description
				if len(desc) > 40 {
					desc = desc[:40]
				}
			}
			fmt.Printf("  [%d/%d] NOW FOUND: %-15s → %s [%s]\n", i+1, len(parts), p, desc, sr.SearchStrategy)
		} else {
			notFound++
			notFoundList = append(notFoundList, p)
			if (i+1)%50 == 0 {
				fmt.Printf("  [%d/%d] progress: %d found, %d still missing\n", i+1, len(parts), found, notFound)
			}
		}
	}

	fmt.Printf("\n=== RESULTS ===\n")
	fmt.Printf("Previously NOT FOUND: %d\n", len(parts))
	fmt.Printf("NOW FOUND:            %d (%.1f%%)\n", found, float64(found)/float64(len(parts))*100)
	fmt.Printf("STILL NOT FOUND:      %d\n", notFound)
	fmt.Printf("ERRORS:               %d\n", errors)
	fmt.Printf("\nStrategy breakdown:\n")
	for s, c := range strategyCount {
		fmt.Printf("  %-30s %d\n", s, c)
	}
	fmt.Printf("\nProjected new accuracy: %.1f%% (%d / %d found)\n",
		float64(2199+found)/float64(2560)*100, 2199+found, 2560)

	// Save still-not-found
	if len(notFoundList) > 0 {
		out, _ := os.Create("still_not_found.txt")
		defer out.Close()
		for _, p := range notFoundList {
			fmt.Fprintln(out, p)
		}
		fmt.Printf("\nStill-not-found parts saved to still_not_found.txt\n")
	}
}
