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

	// Try multiple aftermarket cross-reference sources
	sources := []struct {
		name string
		url  string
	}{
		{"AutoDoc", fmt.Sprintf("https://www.autodoc.co.uk/search?keyword=%s", partNum)},
		{"PartsCatalog", fmt.Sprintf("https://parts-catalog.net/oem/%s/", strings.ReplaceAll(partNum, "-", ""))},
	}

	for _, src := range sources {
		fmt.Printf("\n=== %s ===\n", src.name)
		fmt.Printf("URL: %s\n", src.url)

		req, _ := http.NewRequest("GET", src.url, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
			continue
		}

		fmt.Printf("Status: %d\n", resp.StatusCode)
		fmt.Printf("Final URL: %s\n", resp.Request.URL.String())

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
		resp.Body.Close()
		html := string(body)
		fmt.Printf("HTML size: %d bytes\n", len(html))

		// Save HTML for inspection
		fname := fmt.Sprintf("debug_%s.html", strings.ToLower(src.name))
		os.WriteFile(fname, body, 0644)
		fmt.Printf("Saved to %s\n", fname)

		// Search for brand names
		brands := []string{"MANN-FILTER", "MANN FILTER", "MAHLE", "BOSCH", "DENSO", "NGK", "BREMBO", "TRW",
			"GATES", "SKF", "VALEO", "HELLA", "GMB", "FEBEST", "PURFLUX", "HENGST", "WIX", "CHAMPION", "FRAM",
			"KNECHT", "BLUE PRINT", "NIPPARTS", "JAPANPARTS", "MEYLE", "FEBI"}
		fmt.Println("\nBrand mentions:")
		for _, brand := range brands {
			idx := strings.Index(strings.ToUpper(html), strings.ToUpper(brand))
			if idx >= 0 {
				start := idx - 50
				if start < 0 {
					start = 0
				}
				end := idx + 150
				if end > len(html) {
					end = len(html)
				}
				fmt.Printf("  FOUND: %s at pos %d\n", brand, idx)
				context := html[start:end]
				context = regexp.MustCompile(`\s+`).ReplaceAllString(context, " ")
				fmt.Printf("    Context: ...%s...\n", context)
			}
		}

		// Look for product listings / part numbers
		rePartNum := regexp.MustCompile(`(?i)(OEM|original|cross|ref)[^<]{0,50}(26300|2630035)`)
		matches := rePartNum.FindAllString(html, 10)
		if len(matches) > 0 {
			fmt.Println("\nOEM cross-references found:")
			for _, m := range matches {
				fmt.Printf("  %s\n", m)
			}
		}

		time.Sleep(2 * time.Second) // Be polite between requests
	}
}
