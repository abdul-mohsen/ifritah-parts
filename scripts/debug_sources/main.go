package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

func main() {
	partNum := "26300-35505"
	if len(os.Args) > 1 {
		partNum = os.Args[1]
	}
	client := &http.Client{Timeout: 20 * time.Second}

	// Try multiple sources
	sources := []struct {
		name string
		url  string
	}{
		// partsfinder.eu - TecDoc-based cross-reference lookup
		{"PartsFinder", fmt.Sprintf("https://www.partsfinder.eu/en/catalog/search?q=%s", strings.ReplaceAll(partNum, "-", ""))},
		// onlinecarparts.co.uk - lists aftermarket parts by OEM
		{"OnlineCarParts", fmt.Sprintf("https://www.onlinecarparts.co.uk/search.html?q=%s", strings.ReplaceAll(partNum, "-", ""))},
		// misterauto
		{"MisterAuto", fmt.Sprintf("https://www.mister-auto.co.uk/search?q=%s", strings.ReplaceAll(partNum, "-", ""))},
	}

	for _, src := range sources {
		fmt.Printf("\n=== %s ===\n", src.name)
		fmt.Printf("URL: %s\n", src.url)

		req, _ := http.NewRequest("GET", src.url, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "text/html,application/xhtml+xml")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
			time.Sleep(time.Second)
			continue
		}

		fmt.Printf("Status: %d\n", resp.StatusCode)
		fmt.Printf("Final URL: %s\n", resp.Request.URL)

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
		resp.Body.Close()
		html := string(body)
		fmt.Printf("HTML size: %d bytes\n", len(html))

		// Save
		fname := fmt.Sprintf("debug_%s.html", strings.ToLower(strings.ReplaceAll(src.name, " ", "")))
		os.WriteFile(fname, body, 0644)

		// Quick brand scan
		brands := []string{"MANN-FILTER", "MAHLE", "BOSCH", "DENSO", "HENGST", "WIX", "CHAMPION", "PURFLUX", "KNECHT", "BLUE PRINT", "NIPPARTS"}
		found := 0
		for _, brand := range brands {
			if strings.Contains(strings.ToUpper(html), strings.ToUpper(brand)) {
				found++
				idx := strings.Index(strings.ToUpper(html), strings.ToUpper(brand))
				start := idx - 30
				if start < 0 {
					start = 0
				}
				end := idx + 100
				if end > len(html) {
					end = len(html)
				}
				context := regexp.MustCompile(`\s+`).ReplaceAllString(html[start:end], " ")
				fmt.Printf("  FOUND: %s → %s\n", brand, context)
			}
		}
		if found == 0 {
			fmt.Println("  No aftermarket brands found")
		}

		time.Sleep(2 * time.Second)
	}
}
