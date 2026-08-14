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

// The QA team researches free aftermarket cross-reference websites.
// Each "reviewer" investigates a different category of sources.

type sourceProbe struct {
	name   string
	urlFn  func(oem string) string
	brands []string // brands to search for in response
}

var testParts = []struct {
	oem  string
	desc string
}{
	{"26300-35505", "Oil Filter"},
	{"58101-D3A70", "Brake Pads Front"},
	{"97133-D3000", "Cabin Filter"},
	{"27301-2B100", "Ignition Coil"},
	{"54651-D3000", "Shock Absorber"},
}

var aftermarketBrands = []string{
	"MANN", "MAHLE", "BOSCH", "DENSO", "NGK", "BREMBO", "TRW", "FERODO",
	"TEXTAR", "ATE", "KYB", "SACHS", "MONROE", "BILSTEIN", "GATES",
	"SKF", "VALEO", "HELLA", "MEYLE", "FEBI", "BLUE PRINT", "NIPPARTS",
	"JAPANPARTS", "DELPHI", "CHAMPION", "WIX", "FRAM", "HENGST",
	"PURFLUX", "KNECHT", "NRF", "NISSENS", "MOOG", "CORTECO",
}

func main() {
	client := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	sources := []sourceProbe{
		// === CATEGORY 1: OEM Cross-Reference Databases ===
		{
			name: "Partsfinder.co.uk",
			urlFn: func(oem string) string {
				return fmt.Sprintf("https://www.partsfinder.co.uk/search.php?q=%s", strings.ReplaceAll(oem, "-", ""))
			},
		},
		{
			name: "TecAlliance WebCat (free demo)",
			urlFn: func(oem string) string {
				return fmt.Sprintf("https://web.tecalliance.net/tecdoc-web/search?q=%s", strings.ReplaceAll(oem, "-", ""))
			},
		},
		{
			name: "7zap.com (cross-ref)",
			urlFn: func(oem string) string {
				return fmt.Sprintf("https://7zap.com/en/search/?q=%s", strings.ReplaceAll(oem, "-", ""))
			},
		},
		// === CATEGORY 2: Aftermarket Retailer Sites ===
		{
			name: "autodoc.co.uk",
			urlFn: func(oem string) string {
				return fmt.Sprintf("https://www.autodoc.co.uk/search?keyword=%s", oem)
			},
		},
		{
			name: "buycarparts.co.uk",
			urlFn: func(oem string) string {
				return fmt.Sprintf("https://www.buycarparts.co.uk/search?q=%s", strings.ReplaceAll(oem, "-", ""))
			},
		},
		// === CATEGORY 3: Free Cross-Reference Lookup Tools ===
		{
			name: "cross-references.info",
			urlFn: func(oem string) string {
				return fmt.Sprintf("https://cross-references.info/%s", strings.ReplaceAll(oem, "-", ""))
			},
		},
		{
			name: "partsbrand.com",
			urlFn: func(oem string) string {
				return fmt.Sprintf("https://www.partsbrand.com/search?q=%s", oem)
			},
		},
		// === CATEGORY 4: Parts Catalog Sites ===
		{
			name: "spareto.com",
			urlFn: func(oem string) string {
				return fmt.Sprintf("https://www.spareto.com/search?q=%s", strings.ReplaceAll(oem, "-", ""))
			},
		},
		{
			name: "onlinecarparts.co.uk",
			urlFn: func(oem string) string {
				return fmt.Sprintf("https://www.onlinecarparts.co.uk/search.html?q=%s", strings.ReplaceAll(oem, "-", ""))
			},
		},
		{
			name: "alvadi.ee (Baltic TecDoc)",
			urlFn: func(oem string) string {
				return fmt.Sprintf("https://www.alvadi.ee/en/search?q=%s", strings.ReplaceAll(oem, "-", ""))
			},
		},
		// === CATEGORY 5: Specific Brand Cross-Ref Tools ===
		{
			name: "MANN-FILTER catalog",
			urlFn: func(oem string) string {
				return fmt.Sprintf("https://catalog.mann-filter.com/EU/eng/search/%s", strings.ReplaceAll(oem, "-", ""))
			},
		},
		{
			name: "bosch-automotive.com",
			urlFn: func(oem string) string {
				return fmt.Sprintf("https://www.boschaftermarket.com/xrm/public/api/search?q=%s&market=GB&language=en", strings.ReplaceAll(oem, "-", ""))
			},
		},
		// === CATEGORY 6: Auto Parts Aggregators ===
		{
			name: "kfrfrbrake.en.made-in-china.com",
			urlFn: func(oem string) string {
				return fmt.Sprintf("https://www.made-in-china.com/productSearch.do?word=%s+hyundai", strings.ReplaceAll(oem, "-", "+"))
			},
		},
		{
			name: "megazip.net",
			urlFn: func(oem string) string {
				return fmt.Sprintf("https://www.megazip.net/zapchasti-search?q=%s", strings.ReplaceAll(oem, "-", ""))
			},
		},
		{
			name: "exist.ae (UAE parts)",
			urlFn: func(oem string) string {
				return fmt.Sprintf("https://exist.ae/en/search/?pcode=%s", strings.ReplaceAll(oem, "-", ""))
			},
		},
		{
			name: "emex.ru (intl parts)",
			urlFn: func(oem string) string {
				return fmt.Sprintf("https://emex.ru/products/%s", strings.ReplaceAll(oem, "-", ""))
			},
		},
	}

	// File to save detailed results
	out, _ := os.Create("qa_source_research.txt")
	defer out.Close()
	w := func(s string, args ...interface{}) {
		msg := fmt.Sprintf(s, args...)
		fmt.Print(msg)
		out.WriteString(msg)
	}

	w("╔═══════════════════════════════════════════════════════════╗\n")
	w("║  QA TEAM — AFTERMARKET SOURCE RESEARCH REPORT           ║\n")
	w("╚═══════════════════════════════════════════════════════════╝\n\n")

	type result struct {
		name       string
		status     int
		size       int
		brandsHit  int
		brandList  []string
		blocked    bool
		hasData    bool
		sampleHTML string
	}

	oem := testParts[0].oem // Start with oil filter 26300-35505
	var results []result

	for _, src := range sources {
		w("━━━ Testing: %s ━━━\n", src.name)
		url := src.urlFn(oem)
		w("  URL: %s\n", url)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			w("  ERROR creating request: %v\n\n", err)
			results = append(results, result{name: src.name, blocked: true})
			time.Sleep(time.Second)
			continue
		}

		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")

		resp, err := client.Do(req)
		if err != nil {
			w("  ERROR: %v\n\n", err)
			results = append(results, result{name: src.name, blocked: true})
			time.Sleep(time.Second)
			continue
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
		resp.Body.Close()
		html := string(body)

		r := result{
			name:   src.name,
			status: resp.StatusCode,
			size:   len(html),
		}

		if resp.StatusCode == 403 || resp.StatusCode == 429 {
			r.blocked = true
			w("  BLOCKED (status %d)\n\n", resp.StatusCode)
			results = append(results, r)
			time.Sleep(2 * time.Second)
			continue
		}

		w("  Status: %d, Size: %d bytes\n", resp.StatusCode, len(html))

		// Search for aftermarket brands
		htmlUpper := strings.ToUpper(html)
		for _, brand := range aftermarketBrands {
			if strings.Contains(htmlUpper, strings.ToUpper(brand)) {
				// Verify it's in a product context, not just a footer/menu
				idx := strings.Index(htmlUpper, strings.ToUpper(brand))
				context := ""
				if idx >= 0 {
					start := idx - 40
					if start < 0 {
						start = 0
					}
					end := idx + len(brand) + 80
					if end > len(html) {
						end = len(html)
					}
					context = regexp.MustCompile(`\s+`).ReplaceAllString(html[start:end], " ")
				}
				r.brandsHit++
				r.brandList = append(r.brandList, brand)
				if len(r.sampleHTML) == 0 && len(context) > 0 {
					r.sampleHTML = context
				}
			}
		}

		// Check if there's structured product data
		rePartNum := regexp.MustCompile(`(?i)(part\s*(?:number|no|#)|article|sku|ref)\s*[:\s]*[\w\s-]{4,20}`)
		partMatches := rePartNum.FindAllString(html, 5)

		if r.brandsHit >= 3 {
			r.hasData = true
			w("  ✓ FOUND %d aftermarket brands: %s\n", r.brandsHit, strings.Join(r.brandList, ", "))
			if len(r.sampleHTML) > 0 {
				w("  Sample: %s\n", r.sampleHTML[:min(150, len(r.sampleHTML))])
			}
		} else if r.brandsHit > 0 {
			w("  ~ Found %d brands (weak): %s\n", r.brandsHit, strings.Join(r.brandList, ", "))
		} else {
			w("  ✗ No aftermarket brands found\n")
		}

		if len(partMatches) > 0 {
			w("  Part references found: %s\n", strings.Join(partMatches[:min(3, len(partMatches))], " | "))
		}

		// Save HTML for the promising ones
		if r.hasData {
			fname := fmt.Sprintf("research_%s.html", strings.ReplaceAll(strings.ReplaceAll(strings.ToLower(src.name), " ", "_"), ".", "_"))
			os.WriteFile(fname, body, 0644)
			w("  → Saved HTML as %s\n", fname)
		}

		w("\n")
		results = append(results, r)
		time.Sleep(2 * time.Second) // Be polite
	}

	// === CONSOLIDATED REPORT ===
	w("\n╔═══════════════════════════════════════════════════════════╗\n")
	w("║            SOURCE EVALUATION SUMMARY                     ║\n")
	w("╚═══════════════════════════════════════════════════════════╝\n\n")

	w("┌─────────────────────────────────┬────────┬───────┬────────┬──────────┐\n")
	w("│ Source                          │ Status │ Brands│ Usable │ Verdict  │\n")
	w("├─────────────────────────────────┼────────┼───────┼────────┼──────────┤\n")
	for _, r := range results {
		verdict := "SKIP"
		usable := "No"
		if r.blocked {
			verdict = "BLOCKED"
		} else if r.hasData {
			verdict = "★ USE"
			usable = "YES"
		} else if r.brandsHit > 0 {
			verdict = "WEAK"
		}
		name := r.name
		if len(name) > 31 {
			name = name[:28] + "..."
		}
		w("│ %-31s │ %3d    │ %3d   │ %-6s │ %-8s │\n", name, r.status, r.brandsHit, usable, verdict)
	}
	w("└─────────────────────────────────┴────────┴───────┴────────┴──────────┘\n\n")

	// Now do deep probes on the promising sources with ALL test parts
	var promising []sourceProbe
	for i, r := range results {
		if r.hasData {
			promising = append(promising, sources[i])
		}
	}

	if len(promising) == 0 {
		w("No promising sources found with the first part. Retrying with brake pads...\n\n")
		// Try brake pads too
		oem2 := "58101-D3A70"
		for _, src := range sources {
			if results[0].blocked {
				continue
			}
			url := src.urlFn(oem2)
			req, _ := http.NewRequest("GET", url, nil)
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
			resp, err := client.Do(req)
			if err != nil {
				continue
			}
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
			resp.Body.Close()
			htmlUpper := strings.ToUpper(string(body))
			hits := 0
			for _, brand := range aftermarketBrands {
				if strings.Contains(htmlUpper, strings.ToUpper(brand)) {
					hits++
				}
			}
			if hits >= 3 {
				w("  ★ %s has %d brands for brake pads!\n", src.name, hits)
				promising = append(promising, src)
			}
			time.Sleep(2 * time.Second)
		}
	}

	w("\n═══ DEEP PROBE: Testing %d promising sources with %d parts ═══\n\n", len(promising), len(testParts))

	for _, src := range promising {
		w("━━━━━━ DEEP PROBE: %s ━━━━━━\n", src.name)
		totalBrands := 0
		for _, tp := range testParts {
			url := src.urlFn(tp.oem)
			req, _ := http.NewRequest("GET", url, nil)
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
			req.Header.Set("Accept", "text/html,application/xhtml+xml")
			resp, err := client.Do(req)
			if err != nil {
				w("  %s (%s): ERROR %v\n", tp.oem, tp.desc, err)
				time.Sleep(time.Second)
				continue
			}
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
			resp.Body.Close()
			htmlUpper := strings.ToUpper(string(body))
			var found []string
			for _, brand := range aftermarketBrands {
				if strings.Contains(htmlUpper, strings.ToUpper(brand)) {
					found = append(found, brand)
				}
			}
			totalBrands += len(found)
			if len(found) > 0 {
				w("  %s (%s): %d brands → %s\n", tp.oem, tp.desc, len(found), strings.Join(found, ", "))
			} else {
				w("  %s (%s): No brands found\n", tp.oem, tp.desc)
			}
			time.Sleep(2 * time.Second)
		}
		w("  TOTAL BRANDS ACROSS ALL PARTS: %d\n\n", totalBrands)
	}

	w("\nReport saved to: qa_source_research.txt\n")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
