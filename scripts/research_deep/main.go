package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// Phase 2: Focus on sites that provide ACTUAL per-part cross-reference data.
// The phase 1 probe showed that most "hits" were just brand names in page templates.
// This phase tries cross-reference tool sites, manufacturer lookup APIs, and verified data extractors.

func main() {
	client := &http.Client{
		Timeout: 25 * time.Second,
	}

	out, _ := os.Create("qa_deep_research.txt")
	defer out.Close()
	w := func(s string, args ...interface{}) {
		msg := fmt.Sprintf(s, args...)
		fmt.Print(msg)
		out.WriteString(msg)
	}

	w("╔════════════════════════════════════════════════════════════════╗\n")
	w("║  QA TEAM — PHASE 2 DEEP RESEARCH: ACTUAL CROSS-REF DATA     ║\n")
	w("╚════════════════════════════════════════════════════════════════╝\n\n")

	// Test OEM: 26300-35505 (Hyundai Oil Filter) — very well known cross-refs:
	// MANN W811/80, MAHLE OC205, BOSCH 0986AF1014, WIX WL7171, HENGST H97W05
	testOEM := "2630035505"
	testOEMDash := "26300-35505"

	type probeResult struct {
		name     string
		url      string
		status   int
		hasReal  bool // has actual part-number level data
		found    []string
		snippet  string
		dataType string // "html", "json", "api"
	}

	var results []probeResult

	// Helper
	doReq := func(url string) (int, string, error) {
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,application/json,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		resp, err := client.Do(req)
		if err != nil {
			return 0, "", err
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		resp.Body.Close()
		return resp.StatusCode, string(body), nil
	}

	// ═══ 1. MANN-FILTER Cross Reference API ═══
	w("━━━ 1. MANN-FILTER Online Catalog (direct cross-ref) ━━━\n")
	{
		// MANN has a catalog with OEM cross-ref search
		url := fmt.Sprintf("https://catalog.mann-filter.com/EU/eng/vehicle/search/oe/%s", testOEM)
		status, body, err := doReq(url)
		if err != nil {
			w("  ERROR: %v\n", err)
		} else {
			w("  URL: %s\n  Status: %d, Size: %d\n", url, status, len(body))
			// Look for actual MANN part numbers (format: W 811/80, HU 7008 z, C 28 036, etc.)
			reMann := regexp.MustCompile(`(?i)[WCHUPFL]{1,2}\s?\d{2,5}/?[\d]*\s?[a-z]?`)
			mannParts := reMann.FindAllString(body, 10)
			if len(mannParts) > 0 {
				w("  ★ MANN parts found: %v\n", mannParts)
				results = append(results, probeResult{name: "MANN-FILTER Catalog", url: url, status: status, hasReal: true, found: mannParts, dataType: "html"})
			} else {
				// Try alternate search URL
				url2 := fmt.Sprintf("https://catalog.mann-filter.com/EU/eng/search/%s", testOEM)
				status2, body2, _ := doReq(url2)
				mannParts2 := reMann.FindAllString(body2, 10)
				w("  Alt URL status: %d, MANN parts: %v\n", status2, mannParts2)
				// Look for W811/80 specifically
				if strings.Contains(body2, "W 811") || strings.Contains(body2, "W811") {
					w("  ★ Found W811 reference!\n")
					results = append(results, probeResult{name: "MANN-FILTER Catalog", url: url2, status: status2, hasReal: true, found: []string{"W811"}, dataType: "html"})
				}
			}
		}
	}
	w("\n")
	time.Sleep(2 * time.Second)

	// ═══ 2. WIX Filters Cross Reference ═══
	w("━━━ 2. WIX Filters Cross Reference ━━━\n")
	{
		// WIX has an official cross-reference tool
		url := fmt.Sprintf("https://www.wixfilters.com/catalog/PartSearch.aspx?partnumber=%s", testOEM)
		status, body, err := doReq(url)
		if err != nil {
			w("  ERROR: %v\n", err)
		} else {
			w("  URL: %s\n  Status: %d, Size: %d\n", url, status, len(body))
			reWix := regexp.MustCompile(`(?i)WL?\d{4,6}`)
			wixParts := reWix.FindAllString(body, 10)
			if len(wixParts) > 0 {
				w("  ★ WIX parts found: %v\n", wixParts)
				results = append(results, probeResult{name: "WIX Filters", url: url, status: status, hasReal: true, found: wixParts, dataType: "html"})
			} else {
				w("  No WIX part numbers found in response\n")
			}
		}

		// Also try their API
		url2 := "https://www.wixfilters.com/api/Search/PartSearch?partNumber=" + testOEM
		status2, body2, err2 := doReq(url2)
		if err2 == nil && status2 == 200 {
			w("  API: Status %d, Size: %d\n", status2, len(body2))
			if len(body2) > 10 && body2[0] == '{' || body2[0] == '[' {
				w("  ★ Got JSON response: %s\n", body2[:min(200, len(body2))])
				results = append(results, probeResult{name: "WIX API", url: url2, status: status2, hasReal: true, found: []string{body2[:min(100, len(body2))]}, dataType: "json"})
			}
		}
	}
	w("\n")
	time.Sleep(2 * time.Second)

	// ═══ 3. HENGST Filtration Cross-Reference ═══
	w("━━━ 3. HENGST Filtration ━━━\n")
	{
		url := fmt.Sprintf("https://www.hengst.com/en/catalog/search?q=%s", testOEM)
		status, body, err := doReq(url)
		if err != nil {
			w("  ERROR: %v\n", err)
		} else {
			w("  Status: %d, Size: %d\n", status, len(body))
			reHengst := regexp.MustCompile(`(?i)H\d{2,3}W\d{2,3}`)
			parts := reHengst.FindAllString(body, 10)
			if len(parts) > 0 {
				w("  ★ HENGST parts: %v\n", parts)
			}
			if strings.Contains(body, "H97W05") || strings.Contains(body, "H97W") {
				w("  ★ Found H97W05!\n")
				results = append(results, probeResult{name: "HENGST", url: url, status: status, hasReal: true, found: []string{"H97W05"}, dataType: "html"})
			}
		}
	}
	w("\n")
	time.Sleep(2 * time.Second)

	// ═══ 4. RockAuto.com (huge aftermarket catalog) ═══
	w("━━━ 4. RockAuto.com ━━━\n")
	{
		url := fmt.Sprintf("https://www.rockauto.com/en/partsearch/?partnum=%s", testOEMDash)
		status, body, err := doReq(url)
		if err != nil {
			w("  ERROR: %v\n", err)
		} else {
			w("  Status: %d, Size: %d\n", status, len(body))
			// RockAuto shows many brands
			brands := []string{"MANN", "MAHLE", "WIX", "FRAM", "BOSCH", "DENSO", "HENGST"}
			var found []string
			for _, b := range brands {
				if strings.Contains(strings.ToUpper(body), b) {
					found = append(found, b)
				}
			}
			if len(found) > 0 {
				w("  Brands found: %v\n", found)
			}
			// Look for part numbers
			rePN := regexp.MustCompile(`(?i)(?:W\s?811|OC\s?205|WL\s?7171|PH\s?6811)`)
			pns := rePN.FindAllString(body, 10)
			if len(pns) > 0 {
				w("  ★ Known part numbers found: %v\n", pns)
				results = append(results, probeResult{name: "RockAuto", url: url, status: status, hasReal: true, found: pns, dataType: "html"})
			}
		}
	}
	w("\n")
	time.Sleep(2 * time.Second)

	// ═══ 5. FilterLookup.com ═══
	w("━━━ 5. FilterLookup.com ━━━\n")
	{
		url := fmt.Sprintf("https://www.filterlookup.com/cross-reference/%s", testOEM)
		status, body, err := doReq(url)
		if err != nil {
			w("  ERROR: %v\n", err)
		} else {
			w("  Status: %d, Size: %d\n", status, len(body))
			// Look for known cross-refs
			reCR := regexp.MustCompile(`(?i)(?:W\s?811|OC\s?205|WL\s?7171|BK\s?\d{4}|MANN|MAHLE|FLEETGUARD)`)
			crs := reCR.FindAllString(body, 20)
			if len(crs) > 0 {
				w("  ★ Cross-refs: %v\n", crs)
				results = append(results, probeResult{name: "FilterLookup", url: url, status: status, hasReal: true, found: crs, dataType: "html"})
			}
		}
		// Also try alternate URL patterns
		url2 := fmt.Sprintf("https://www.filterlookup.com/search?q=%s", testOEMDash)
		status2, body2, _ := doReq(url2)
		if status2 == 200 {
			w("  Alt search: Status %d, Size: %d\n", status2, len(body2))
		}
	}
	w("\n")
	time.Sleep(2 * time.Second)

	// ═══ 6. NGK/NTK Part Finder ═══
	w("━━━ 6. NGK/NTK Catalog ━━━\n")
	{
		// Test with ignition coil 27301-2B100
		url := fmt.Sprintf("https://www.ngk.com/search?q=%s", "273012B100")
		status, body, err := doReq(url)
		if err != nil {
			w("  ERROR: %v\n", err)
		} else {
			w("  Status: %d, Size: %d\n", status, len(body))
			reNGK := regexp.MustCompile(`(?i)(U\d{4,5}|BKR\d[A-Z]{1,4}|IKR\d[A-Z]{0,4})`)
			parts := reNGK.FindAllString(body, 10)
			if len(parts) > 0 {
				w("  ★ NGK parts: %v\n", parts)
				results = append(results, probeResult{name: "NGK", url: url, status: status, hasReal: true, found: parts, dataType: "html"})
			}
		}
	}
	w("\n")
	time.Sleep(2 * time.Second)

	// ═══ 7. bfrparts.com ═══
	w("━━━ 7. bfrparts.com ━━━\n")
	{
		url := fmt.Sprintf("https://bfrparts.com/index.php?route=product/search&search=%s", testOEMDash)
		status, body, err := doReq(url)
		if err != nil {
			w("  ERROR: %v\n", err)
		} else {
			w("  Status: %d, Size: %d\n", status, len(body))
			reParts := regexp.MustCompile(`(?i)(W\s?811|OC\s?205|MANN|MAHLE|WIX|HENGST|blue\s?print)`)
			parts := reParts.FindAllString(body, 20)
			if len(parts) > 0 {
				w("  ★ Parts/brands: %v\n", parts)
				results = append(results, probeResult{name: "bfrparts.com", url: url, status: status, hasReal: true, found: parts, dataType: "html"})
			}
		}
	}
	w("\n")
	time.Sleep(2 * time.Second)

	// ═══ 8. filtersfast.com (cross-reference) ═══
	w("━━━ 8. FiltersDB / Donaldson Cross-Ref ━══\n")
	{
		url := fmt.Sprintf("https://www.donaldson.com/en-us/industrial/filters/cross-reference/?q=%s", testOEMDash)
		status, body, err := doReq(url)
		if err != nil {
			w("  ERROR: %v\n", err)
		} else {
			w("  Status: %d, Size: %d\n", status, len(body))
			if strings.Contains(body, "P55") || strings.Contains(body, "cross-reference") {
				w("  Has cross-reference content\n")
			}
		}
	}
	w("\n")
	time.Sleep(2 * time.Second)

	// ═══ 9. partsfinder.eu ═══
	w("━━━ 9. partsfinder.eu ━━━\n")
	{
		url := fmt.Sprintf("https://www.partsfinder.eu/search?q=%s", testOEM)
		status, body, err := doReq(url)
		if err != nil {
			w("  ERROR: %v\n", err)
		} else {
			w("  Status: %d, Size: %d\n", status, len(body))
			brands := []string{"MANN", "MAHLE", "BOSCH", "WIX", "FRAM", "HENGST", "KNECHT", "BLUE PRINT"}
			var found []string
			for _, b := range brands {
				if strings.Contains(strings.ToUpper(body), b) {
					found = append(found, b)
				}
			}
			if len(found) >= 2 {
				w("  ★ Found brands: %v\n", found)
				results = append(results, probeResult{name: "partsfinder.eu", url: url, status: status, hasReal: true, found: found, dataType: "html"})
			}
		}
	}
	w("\n")
	time.Sleep(2 * time.Second)

	// ═══ 10. TecDoc free catalog VIA autopartspro.co.uk ═══
	w("━━━ 10. autopartspro.co.uk (TecDoc-backed) ━━━\n")
	{
		url := fmt.Sprintf("https://www.autopartspro.co.uk/search?q=%s", testOEMDash)
		status, body, err := doReq(url)
		if err != nil {
			w("  ERROR: %v\n", err)
		} else {
			w("  Status: %d, Size: %d\n", status, len(body))
			rePN := regexp.MustCompile(`(?i)(W\s?811|OC\s?205|WL\s?7171)`)
			pns := rePN.FindAllString(body, 10)
			if len(pns) > 0 {
				w("  ★ Part numbers: %v\n", pns)
			}
		}
	}
	w("\n")
	time.Sleep(2 * time.Second)

	// ═══ 11. autodarts/parts24 ═══
	w("━━━ 11. parts24.com ═━━\n")
	{
		url := fmt.Sprintf("https://www.parts24.com/en/search?term=%s", testOEM)
		status, body, err := doReq(url)
		if err != nil {
			w("  ERROR: %v\n", err)
		} else {
			w("  Status: %d, Size: %d\n", status, len(body))
			if len(body) > 200 {
				brands := []string{"MANN", "MAHLE", "BOSCH", "WIX", "HENGST", "KNECHT"}
				for _, b := range brands {
					if strings.Contains(strings.ToUpper(body), b) {
						w("  Found: %s\n", b)
					}
				}
			}
		}
	}
	w("\n")
	time.Sleep(2 * time.Second)

	// ═══ 12. cararac.com (Mid-East parts hub) ═══
	w("━━━ 12. catalogs / autodoc API (JSON) ━━━\n")
	{
		// Some sites have JSON APIs behind their search
		urls := []string{
			fmt.Sprintf("https://webapi.autodoc.de/api/atd/search?searchstring=%s&lang=en&page=1", testOEM),
			fmt.Sprintf("https://api.tecalliance.services/catalogs/1/articles/byOeNumber/%s?lang=en", testOEM),
			fmt.Sprintf("https://www.auto-doc.fr/api/v1/search?q=%s", testOEM),
		}
		for _, url := range urls {
			status, body, err := doReq(url)
			if err != nil {
				w("  %s → ERROR: %v\n", url[:50], err)
				continue
			}
			w("  %s → %d (%d bytes)\n", url[:60], status, len(body))
			if status == 200 && (body[0] == '{' || body[0] == '[') {
				var js interface{}
				if json.Unmarshal([]byte(body), &js) == nil {
					snippet := body
					if len(snippet) > 300 {
						snippet = snippet[:300]
					}
					w("  ★ Valid JSON: %s\n", snippet)
					results = append(results, probeResult{name: url[:40], url: url, status: status, hasReal: true, found: []string{snippet}, dataType: "json"})
				}
			}
		}
	}
	w("\n")

	// ═══ SUMMARY ═══
	w("\n╔════════════════════════════════════════════════════════════════╗\n")
	w("║                    FINAL RESULTS                              ║\n")
	w("╚════════════════════════════════════════════════════════════════╝\n\n")

	if len(results) == 0 {
		w("No sources returned actual cross-reference part data.\n")
		w("Next approach: Try TecDoc-Web with proper ajax/API approach,\n")
		w("or build static cross-ref dataset from manufacturer catalogs.\n")
	} else {
		for i, r := range results {
			icon := "★"
			if r.dataType == "json" {
				icon = "◆"
			}
			w("%s %d. %s (%s)\n", icon, i+1, r.name, r.dataType)
			w("   URL: %s\n", r.url)
			w("   Status: %d\n", r.status)
			if len(r.found) > 0 {
				w("   Data: %v\n", r.found[:min(5, len(r.found))])
			}
			w("\n")
		}
	}
	w("\nReport saved to: qa_deep_research.txt\n")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
