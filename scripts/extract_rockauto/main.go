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

// Extract actual aftermarket cross-reference data from RockAuto.com
// RockAuto shows brand + part number for OEM-compatible parts

var testParts = []struct {
	oem  string
	desc string
}{
	{"26300-35505", "Oil Filter"},
	{"28113-D3100", "Air Filter"},
	{"97133-D3000", "Cabin Filter"},
	{"31112-1R000", "Fuel Filter"},
	{"58101-D3A70", "Brake Pads Front"},
	{"51712-D7000", "Brake Disc Front"},
	{"18855-10080", "Spark Plug"},
	{"27301-2B100", "Ignition Coil"},
	{"54651-D3000", "Shock Absorber Front"},
	{"25313-D3500", "Radiator"},
	{"25100-2B700", "Water Pump"},
	{"23060-21030", "Fuel Injector"},
	{"39210-2B220", "O2 Sensor"},
	{"37300-2B100", "Alternator"},
	{"97701-D3000", "A/C Compressor"},
	{"98350-D3000", "Wiper Blade"},
	{"51720-D3000", "Wheel Bearing Front"},
	{"56820-3X000", "Tie Rod End"},
	{"54500-D3000", "Control Arm Front"},
	{"54830-D3000", "Stabilizer Link"},
}

func main() {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	out, _ := os.Create("rockauto_crossref_results.txt")
	defer out.Close()
	w := func(s string, args ...interface{}) {
		msg := fmt.Sprintf(s, args...)
		fmt.Print(msg)
		out.WriteString(msg)
	}

	w("╔═══════════════════════════════════════════════════════════════════╗\n")
	w("║  ROCKAUTO CROSS-REFERENCE EXTRACTION — ALL TEST PARTS           ║\n")
	w("╚═══════════════════════════════════════════════════════════════════╝\n\n")

	// Patterns for aftermarket part numbers by brand
	partPatterns := map[string]*regexp.Regexp{
		// Generic: brand name followed by part number
		"BRAND_EXTRACTION": regexp.MustCompile(`(?i)(MANN-FILTER|MANN[\s+]FILTER|MAHLE|BOSCH|DENSO|NGK|BREMBO|TRW|FERODO|TEXTAR|ATE|KYB|SACHS|MONROE|BILSTEIN|GATES|SKF|VALEO|HELLA|MEYLE|FEBI|BLUE\s?PRINT|NIPPARTS|DELPHI|CHAMPION|WIX|FRAM|HENGST|PURFLUX|KNECHT|NRF|NISSENS|MOOG|CORTECO|BECK[/\s]ARNLEY|PREMIUM\s?GUARD|STP|ACDELCO|MOTORCRAFT|PUROLATOR|PRONTO|MICRO[G]?ARD|GP\s?SORENSEN|STANDARD|SPECTRA|TIMKEN|NATIONAL|BCA|CARDONE|WAI|BBB\s?Industries)`),
	}
	_ = partPatterns

	// Extract structured data from RockAuto HTML
	// RockAuto uses class="listing-text-row" or similar patterns
	reListingBrand := regexp.MustCompile(`(?i)class="[^"]*listing[^"]*"[^>]*>\s*<[^>]*>([^<]{2,40})</`)
	rePartNumber := regexp.MustCompile(`(?i)class="[^"]*(?:listing-final-manufacturer|ra-btn-partno|listing-text-row-partno)[^"]*"[^>]*>([^<]{3,30})</`)
	reBrandPN := regexp.MustCompile(`(?i)<span[^>]*class="[^"]*ra-btn-mfr[^"]*"[^>]*>([^<]+)</span>\s*<span[^>]*class="[^"]*ra-btn-part[^"]*"[^>]*>([^<]+)</span>`)

	// Broader pattern: look for table rows with brand + part info
	reListing := regexp.MustCompile(`(?is)<tr[^>]*class="[^"]*listing[^"]*"[^>]*>(.*?)</tr>`)
	reAnyBrand := regexp.MustCompile(`(?i)(MANN.?FILTER|MAHLE|BOSCH|DENSO|NGK|BREMBO|TRW|FERODO|TEXTAR|ATE|KYB|SACHS|MONROE|BILSTEIN|GATES|SKF|VALEO|HELLA|MEYLE|FEBI|BLUE.?PRINT|NIPPARTS|DELPHI|CHAMPION|WIX|FRAM|HENGST|PURFLUX|KNECHT|NRF|NISSENS|MOOG|CORTECO|BECK.?ARNLEY|PREMIUM.?GUARD|STP|ACDELCO|MOTORCRAFT|PUROLATOR|PRONTO|MICRO.?ARD|STANDARD|SPECTRA|TIMKEN|CARDONE|WAI|BBB|CENTRIC|ACL|DNJ|ITM|HASTINGS|CLEVITE|SEALED.?POWER|ROL|FEL.?PRO|AIRTEX|US.?MOTOR|DORMAN|DAYCO)`)

	// Approach: look for brand+partnumber in product listing blocks
	reProduct := regexp.MustCompile(`(?is)<(?:div|td|li|a|span)[^>]*(?:class|id)="[^"]*(?:listing|product|result|item|part)[^"]*"[^>]*>(.{10,500}?)</(?:div|td|li|a|span)>`)

	type crossRef struct {
		brand   string
		partNum string
	}

	totalFound := 0
	totalParts := 0

	for _, tp := range testParts {
		url := fmt.Sprintf("https://www.rockauto.com/en/partsearch/?partnum=%s", tp.oem)

		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		req.Header.Set("Referer", "https://www.rockauto.com/")

		resp, err := client.Do(req)
		if err != nil {
			w("✗ %s (%s): ERROR %v\n", tp.oem, tp.desc, err)
			time.Sleep(3 * time.Second)
			continue
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
		resp.Body.Close()
		html := string(body)

		if resp.StatusCode != 200 {
			w("✗ %s (%s): HTTP %d\n", tp.oem, tp.desc, resp.StatusCode)
			time.Sleep(3 * time.Second)
			continue
		}

		// Save HTML for first part for analysis
		if tp.oem == "26300-35505" {
			os.WriteFile("rockauto_sample.html", body, 0644)
		}

		var refs []crossRef
		seen := map[string]bool{}

		// Method 1: Brand+PN button pattern
		matches1 := reBrandPN.FindAllStringSubmatch(html, -1)
		for _, m := range matches1 {
			brand := strings.TrimSpace(m[1])
			pn := strings.TrimSpace(m[2])
			key := brand + "|" + pn
			if !seen[key] {
				seen[key] = true
				refs = append(refs, crossRef{brand, pn})
			}
		}

		// Method 2: Listing rows with brand extraction
		listings := reListing.FindAllStringSubmatch(html, -1)
		for _, m := range listings {
			bloc := m[1]
			brandMatch := reAnyBrand.FindString(bloc)
			if brandMatch != "" {
				// Extract part number from same block
				pnMatch := rePartNumber.FindStringSubmatch(bloc)
				if pnMatch != nil {
					key := brandMatch + "|" + pnMatch[1]
					if !seen[key] {
						seen[key] = true
						refs = append(refs, crossRef{brandMatch, pnMatch[1]})
					}
				}
			}
		}

		// Method 3: Product blocks with brand
		products := reProduct.FindAllStringSubmatch(html, -1)
		for _, m := range products {
			bloc := m[1]
			brandMatch := reAnyBrand.FindString(bloc)
			if brandMatch != "" {
				// Try to find a part number nearby
				rePN := regexp.MustCompile(`(?i)([A-Z]{1,4}\s?\d{3,7}[/]?\d{0,3}\s?[A-Z]{0,3})`)
				pnMatches := rePN.FindAllString(bloc, 5)
				for _, pn := range pnMatches {
					pn = strings.TrimSpace(pn)
					if len(pn) >= 4 && !strings.EqualFold(pn, brandMatch) {
						key := brandMatch + "|" + pn
						if !seen[key] {
							seen[key] = true
							refs = append(refs, crossRef{brandMatch, pn})
						}
					}
				}
			}
		}

		// Method 4: Find brands in HTML and extract surrounding part numbers
		brandMatches := reAnyBrand.FindAllStringIndex(html, -1)
		_ = reListingBrand
		for _, idx := range brandMatches {
			brand := html[idx[0]:idx[1]]
			// Look 200 chars after brand name for a part number
			endCtx := idx[1] + 200
			if endCtx > len(html) {
				endCtx = len(html)
			}
			context := html[idx[1]:endCtx]
			// Extract alphanumeric part numbers (3-20 chars)
			rePNCtx := regexp.MustCompile(`[A-Z0-9][A-Z0-9\s/-]{2,18}[A-Z0-9]`)
			ctxPNs := rePNCtx.FindAllString(context, 3)
			for _, pn := range ctxPNs {
				pn = strings.TrimSpace(pn)
				if len(pn) >= 4 && len(pn) <= 20 {
					key := brand + "|" + pn
					if !seen[key] {
						seen[key] = true
						refs = append(refs, crossRef{brand, pn})
					}
				}
			}
		}

		totalParts++
		totalFound += len(refs)

		if len(refs) > 0 {
			w("★ %s (%s) — %d cross-refs:\n", tp.oem, tp.desc, len(refs))
			for _, r := range refs {
				w("    %s → %s\n", r.brand, r.partNum)
			}
		} else {
			// Show what brands we found at all
			allBrands := reAnyBrand.FindAllString(html, -1)
			uniqueBrands := map[string]bool{}
			for _, b := range allBrands {
				uniqueBrands[strings.ToUpper(b)] = true
			}
			brandList := make([]string, 0, len(uniqueBrands))
			for b := range uniqueBrands {
				brandList = append(brandList, b)
			}
			w("○ %s (%s) — 0 part-level refs (brands seen: %s)\n", tp.oem, tp.desc, strings.Join(brandList, ", "))

			// Save HTML for debugging
			fname := fmt.Sprintf("rockauto_%s.html", strings.ReplaceAll(tp.oem, "-", ""))
			os.WriteFile(fname, body, 0644)
			w("  → Saved as %s for analysis\n", fname)
		}
		w("\n")

		time.Sleep(4 * time.Second) // Be respectful
	}

	w("\n═══════════════════════════════════════════\n")
	w("TOTAL: %d cross-refs from %d parts tested\n", totalFound, totalParts)
	w("═══════════════════════════════════════════\n")
}
