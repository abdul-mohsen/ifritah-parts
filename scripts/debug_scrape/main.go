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
	// Test with oil filter 26300-35505 - should have MANN-FILTER, MAHLE, BOSCH etc.
	partNum := "2630035505"
	if len(os.Args) > 1 {
		partNum = os.Args[1]
	}

	client := &http.Client{Timeout: 15 * time.Second}
	url := fmt.Sprintf("https://partsouq.com/en/search/all?q=%s", partNum)
	fmt.Println("Fetching:", url)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("ERROR:", err)
		return
	}
	defer resp.Body.Close()

	fmt.Println("Status:", resp.StatusCode)

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	html := string(body)

	fmt.Printf("HTML length: %d bytes\n", len(html))
	fmt.Println("---")

	// Save full HTML for inspection
	os.WriteFile("debug_partsouq.html", body, 0644)
	fmt.Println("Saved full HTML to debug_partsouq.html")

	// Look for brand names in the HTML
	brands := []string{"MANN", "MAHLE", "BOSCH", "PURFLUX", "HENGST", "WIX", "CHAMPION", "FRAM", "UFI", "Mobis", "Mando", "GMB", "FEBEST", "Gates"}
	fmt.Println("\n--- BRAND MENTIONS IN HTML ---")
	for _, brand := range brands {
		count := strings.Count(strings.ToUpper(html), strings.ToUpper(brand))
		if count > 0 {
			fmt.Printf("  %s: %d occurrences\n", brand, count)
			// Find first occurrence context
			idx := strings.Index(strings.ToUpper(html), strings.ToUpper(brand))
			start := idx - 100
			if start < 0 {
				start = 0
			}
			end := idx + 200
			if end > len(html) {
				end = len(html)
			}
			fmt.Printf("  Context: ...%s...\n\n", html[start:end])
		}
	}

	// Try the regex
	reAftermarket := regexp.MustCompile(`(?i)(` +
		`Mobis|Mando|ICRBI|Sure|Parts Mall|CTR|ONNURI|AMD|Korean Stars|` +
		`MANN-FILTER|MANN FILTER|MAHLE|KNECHT|BOSCH|PURFLUX|HENGST|CHAMPION|FRAM|WIX|UFI|KOLBENSCHMIDT|` +
		`BREMBO|TRW|FERODO|JURID|ATE|PAGID|TEXTAR|MINTEX|EBC|ZIMMERMANN|BREMSI|` +
		`SACHS|MONROE|BILSTEIN|KYB|KONI|MEYLE|FEBI|FEBI BILSTEIN|LEMFORDER|MOOG|OPTIMAL|SWAG|TOPRAN|SIDEM|DELPHI|FIRST LINE|QUINTON HAZELL|` +
		`VALEO|HELLA|OSRAM|PHILIPS|CONTINENTAL|VEMO|` +
		`GATES|SKF|FAG|INA|SNR|NTN|KOYO|DAYCO|CONTITECH|CORTECO|ELRING|VICTOR REINZ|` +
		`BEHR|NISSENS|NRF|DENSO|PRASCO|DEPO|DIEDERICHS|` +
		`BLUE PRINT|BLUEPRINT ADL|NIPPARTS|JAPANPARTS|ASHIKA|COMLINE|BLUEPRINT|PIERBURG|WAHLER|` +
		`NGK|AISIN|TOKICO|HITACHI|AKEBONO|ADVICS|GMB|555|MASUMA|FEBEST|` +
		`ACDELCO|AC DELCO|MOTORCRAFT|DORMAN|CARDONE` +
		`)\s+(\d[\w-]*)\s+([A-Z][\w\s&/.,-]+?)(?:\s*[<"\]])`)

	matches := reAftermarket.FindAllStringSubmatch(html, -1)
	fmt.Printf("\n--- REGEX MATCHES: %d ---\n", len(matches))
	for i, m := range matches {
		fmt.Printf("  %d: Brand=%q PN=%q Desc=%q\n", i+1, m[1], m[2], m[3])
	}

	// Also try a simpler regex to find any brand+number patterns
	reSimple := regexp.MustCompile(`(?i)(MANN-FILTER|MAHLE|BOSCH|DENSO|NGK|BREMBO|TRW|GATES|SKF|VALEO|HELLA|GMB|FEBEST|Mobis|Mando)\s+\S+`)
	simpleMatches := reSimple.FindAllString(html, -1)
	fmt.Printf("\n--- SIMPLE BRAND+WORD MATCHES: %d ---\n", len(simpleMatches))
	for _, m := range simpleMatches {
		if len(m) > 100 {
			m = m[:100]
		}
		fmt.Printf("  %s\n", m)
	}

	// Look for data-number attributes
	reDataNum := regexp.MustCompile(`data-number='([^']+)'`)
	dataMatches := reDataNum.FindAllStringSubmatch(html, -1)
	fmt.Printf("\n--- data-number ATTRIBUTES: %d ---\n", len(dataMatches))
	for _, m := range dataMatches {
		fmt.Printf("  %s\n", m[1])
	}

	// Look for "cross" or "aftermarket" or "alternative" text
	fmt.Println("\n--- SEARCHING FOR CROSS/AFTERMARKET/ALTERNATIVE KEYWORDS ---")
	keywords := []string{"cross", "aftermarket", "alternative", "analogue", "analog", "substitute", "replacement", "similar"}
	for _, kw := range keywords {
		idx := strings.Index(strings.ToLower(html), kw)
		if idx >= 0 {
			start := idx - 50
			if start < 0 {
				start = 0
			}
			end := idx + 100
			if end > len(html) {
				end = len(html)
			}
			fmt.Printf("  '%s' found at %d: ...%s...\n", kw, idx, html[start:end])
		}
	}
}
