package service

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"parts-engine/internal/model"
)

// PartsOuqService provides online part lookups via PartsOuq.com HTML scraping.
type PartsOuqService struct {
	client    *http.Client
	mu        sync.Mutex
	lastFetch time.Time
	cache     *PartsCache
}

// NewPartsOuqService creates a new online lookup service.
// Pass cache=nil to disable caching.
func NewPartsOuqService(cache *PartsCache) *PartsOuqService {
	return &PartsOuqService{
		client: &http.Client{Timeout: 15 * time.Second},
		cache:  cache,
	}
}

// LookupPart fetches part info from PartsOuq.com for a given OEM part number.
// Returns ALL OEM parts found on the page (queried part + substitutions as full results).
func (p *PartsOuqService) LookupPart(partNumber string) ([]*model.OnlinePartResult, error) {
	normalized := normalizePartNumber(partNumber)
	if normalized == "" {
		return nil, fmt.Errorf("empty part number")
	}

	// Check cache first
	if p.cache != nil {
		if cached := p.cache.GetCached(normalized); cached != nil {
			if cached.Source == "not_found" {
				return nil, fmt.Errorf("previously not found on partsouq: %s", partNumber)
			}
			return []*model.OnlinePartResult{cached}, nil
		}
	}

	// Rate limit: min 1 second between requests
	p.mu.Lock()
	elapsed := time.Since(p.lastFetch)
	if elapsed < time.Second {
		p.mu.Unlock()
		time.Sleep(time.Second - elapsed)
		p.mu.Lock()
	}
	p.lastFetch = time.Now()
	p.mu.Unlock()

	url := fmt.Sprintf("https://partsouq.com/en/search/all?q=%s", normalized)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching partsouq: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("partsouq returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024)) // 512KB max
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	results := parseAllPartsFromHTML(string(body), normalized)
	if len(results) == 0 {
		// Store negative cache so we don't re-fetch this
		if p.cache != nil {
			p.cache.StoreNegative(normalized)
		}
		return nil, fmt.Errorf("no results found on partsouq for %s", partNumber)
	}

	for _, result := range results {
		// Decode OEM prefix for category
		if decoded := DecodeOEMPrefix(result.PartNumber); decoded != nil {
			result.Category = decoded.System + " / " + decoded.Category
		}
		result.Source = "partsouq"

		// Store each result in cache individually
		if p.cache != nil {
			if err := p.cache.StoreCache(result.PartNumber, result); err != nil {
				log.Printf("partsouq cache store error: %v", err)
			}
		}
	}

	return results, nil
}

// GetCache returns the underlying parts cache for reverse lookups.
func (p *PartsOuqService) GetCache() *PartsCache {
	return p.cache
}

// normalizePartNumber strips dashes, spaces, dots and uppercases.
func normalizePartNumber(pn string) string {
	var b strings.Builder
	for _, c := range strings.ToUpper(pn) {
		if (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// ── HTML parsing ────────────────────────────────────────────────────

// Pre-compiled regex patterns for parsing PartsOuq search results HTML.
var (
	// Matches the first H1-like description
	reH1Desc = regexp.MustCompile(`<h1[^>]*>\s*(.+?)\s*</h1>`)

	// Matches Make field: Make: ... <a ...>Hyundai / KIA</a>
	reMake = regexp.MustCompile(`(?i)Make:.*?>(Hyundai\s*/\s*KIA|[A-Za-z /]+?)</a>`)

	// Matches substitution entries: "Hyundai / KIA {PartNum} {Description}"
	reSubstitution = regexp.MustCompile(`(?i)(Hyundai\s*/\s*KIA)\s+(\w[\w-]*)\s+([A-Z][\w\s&/-]+?)(?:\s*[<"\]])`)

	// Matches aftermarket entries: Brand PartNumber Description
	// Includes Korean OEM (Mobis, Mando, CTR, Parts Mall, ONNURI, Korean Stars, AMD, Sure, ICRBI),
	// European (MANN-FILTER, MAHLE, BOSCH, BREMBO, TRW, VALEO, SACHS, MEYLE, FEBI, LEMFÖRDER,
	//   HELLA, OSRAM, PHILIPS, CONTINENTAL, GATES, SKF, FAG, INA, SNR, DAYCO, MONROE, PURFLUX,
	//   DELPHI, BLUE PRINT, NIPPARTS, JAPANPARTS, ASHIKA, OPTIMAL, SWAG, TOPRAN, CORTECO, ELRING,
	//   PIERBURG, BEHR, NISSENS, NRF, PRASCO, DEPO, TYCO, FERODO, JURID, ATE, PAGID, TEXTAR,
	//   QUINTON HAZELL, FIRST LINE, COMLINE, BLUEPRINT ADL),
	// Japanese/global (NGK, DENSO, AISIN, NTN, KOYO, KYB, TOKICO, HITACHI, AKEBONO, ADVICS,
	//   GMB, 555, MASUMA, FEBEST)
	reAftermarket = regexp.MustCompile(`(?i)(` +
		// Korean OEM / aftermarket
		`Mobis|Mando|ICRBI|Sure|Parts Mall|CTR|ONNURI|AMD|Korean Stars|` +
		// European — Filtration / Engine
		`MANN-FILTER|MANN FILTER|MAHLE|KNECHT|BOSCH|PURFLUX|HENGST|CHAMPION|FRAM|WIX|UFI|KOLBENSCHMIDT|` +
		// European — Brakes
		`BREMBO|TRW|FERODO|JURID|ATE|PAGID|TEXTAR|MINTEX|EBC|ZIMMERMANN|BREMSI|` +
		// European — Suspension / Steering
		`SACHS|MONROE|BILSTEIN|KYB|KONI|MEYLE|FEBI|FEBI BILSTEIN|` + "LEMF" + `ÖRDER|LEMFORDER|MOOG|OPTIMAL|SWAG|TOPRAN|SIDEM|DELPHI|FIRST LINE|QUINTON HAZELL|` +
		// European — Electrical / Lighting
		`VALEO|HELLA|OSRAM|PHILIPS|CONTINENTAL|VEMO|` +
		// European — Belts / Bearings / Seals
		`GATES|SKF|FAG|INA|SNR|NTN|KOYO|DAYCO|CONTITECH|CORTECO|ELRING|VICTOR REINZ|` +
		// European — Cooling / Climate
		`BEHR|NISSENS|NRF|DENSO|PRASCO|DEPO|DIEDERICHS|` +
		// European — Other
		`BLUE PRINT|BLUEPRINT ADL|NIPPARTS|JAPANPARTS|ASHIKA|COMLINE|BLUEPRINT|PIERBURG|WAHLER|` +
		// Japanese / Global
		`NGK|AISIN|TOKICO|HITACHI|AKEBONO|ADVICS|GMB|555|MASUMA|FEBEST|` +
		// American / Other
		`ACDELCO|AC DELCO|MOTORCRAFT|DORMAN|CARDONE` +
		`)\s+(\d[\w-]*)\s+([A-Z][\w\s&/.,-]+?)(?:\s*[<"\]])`)

	// Matches <li> content (possibly wrapped in <strong>)
	reLiContent = regexp.MustCompile(`<li[^>]*>\s*(?:<strong>)?\s*([^<]+?)(?:</strong>)?\s*(?:</li>|<li|</ul>|$)`)

	// Matches data-number attribute to scope sections to a specific part
	reDataNumber = regexp.MustCompile(`data-number='([^']+)'`)

	// For stripping HTML tags
	reStripTags = regexp.MustCompile(`<[^>]*>`)
)

// parseAllPartsFromHTML extracts ALL OEM parts shown on a PartsOuq search page.
// Each data-number section becomes a full result with its own description, aftermarket, and compatibility.
func parseAllPartsFromHTML(html, queryNumber string) []*model.OnlinePartResult {
	// Discover all unique OEM part numbers on the page via data-number attributes
	partNumbers := reDataNumber.FindAllStringSubmatch(html, 20)
	if len(partNumbers) == 0 {
		// Fallback: try parsing just the queried part the old way
		if r := parseSinglePart(html, queryNumber); r != nil {
			return []*model.OnlinePartResult{r}
		}
		return nil
	}

	seen := make(map[string]bool)
	var ordered []string
	// Queried part first
	for _, m := range partNumbers {
		pn := normalizePartNumber(m[1])
		if pn == queryNumber && !seen[pn] {
			seen[pn] = true
			ordered = append(ordered, pn)
		}
	}
	// Then other parts
	for _, m := range partNumbers {
		pn := normalizePartNumber(m[1])
		if !seen[pn] {
			seen[pn] = true
			ordered = append(ordered, pn)
		}
	}

	// Extract shared page-level info
	make_ := extractMake(html)

	var results []*model.OnlinePartResult
	for _, pn := range ordered {
		r := parseSinglePart(html, pn)
		if r == nil {
			continue
		}
		if r.Make == "" {
			r.Make = make_
		}
		results = append(results, r)
	}

	// If we got results, remove substitution entries that are already full results
	if len(results) > 1 {
		fullParts := make(map[string]bool)
		for _, r := range results {
			fullParts[normalizePartNumber(r.PartNumber)] = true
		}
		for _, r := range results {
			var filtered []model.SubstitutionPart
			for _, s := range r.Substitutions {
				if !fullParts[normalizePartNumber(s.PartNumber)] {
					filtered = append(filtered, s)
				}
			}
			r.Substitutions = filtered
		}
	}

	return results
}

// extractMake gets the make from the page (shared across all parts).
func extractMake(html string) string {
	makeMatches := reMake.FindAllStringSubmatch(html, 10)
	for _, m := range makeMatches {
		mk := strings.TrimSpace(m[1])
		if strings.Contains(strings.ToUpper(mk), "HYUNDAI") || strings.Contains(strings.ToUpper(mk), "KIA") {
			return mk
		}
	}
	if strings.Contains(html, "Hyundai / KIA") || strings.Contains(html, "Hyundai/KIA") {
		return "Hyundai / KIA"
	}
	return ""
}

// parseSinglePart extracts a single OEM part result from the HTML, scoped to partNumber.
func parseSinglePart(html, partNumber string) *model.OnlinePartResult {
	result := &model.OnlinePartResult{
		PartNumber: partNumber,
	}

	// 1. Extract description — look for "Make PartNum DESCRIPTION" pattern
	{
		pattern := regexp.MustCompile(`(?i)Hyundai\s*/\s*KIA\s+` + regexp.QuoteMeta(partNumber) + `\s+([A-Z][\w\s&/-]+?)(?:\s*[<"\]])`)
		if m := pattern.FindStringSubmatch(html); len(m) > 1 {
			result.Description = strings.TrimSpace(m[1])
		}
	}
	// Fallback: look for description in product section near this part number
	if result.Description == "" {
		section := findProductSection(html, partNumber)
		if section != "" {
			if m := reH1Desc.FindStringSubmatch(section); len(m) > 1 {
				desc := strings.TrimSpace(stripTags(m[1]))
				if len(desc) > 2 && !strings.EqualFold(desc, "Search") && !strings.Contains(strings.ToLower(desc), "partsouq") {
					result.Description = desc
				}
			}
		}
	}
	// Last fallback: page-level H1
	if result.Description == "" {
		if m := reH1Desc.FindStringSubmatch(html); len(m) > 1 {
			desc := strings.TrimSpace(stripTags(m[1]))
			if len(desc) > 2 && !strings.EqualFold(desc, "Search") && !strings.Contains(strings.ToLower(desc), "partsouq") {
				result.Description = desc
			}
		}
	}

	// Humanize condensed descriptions
	if result.Description != "" {
		result.Description = humanizeDescription(result.Description)
	}

	// 2. Extract make
	result.Make = extractMake(html)

	if result.Description == "" && result.Make == "" {
		return nil
	}

	// 3. Parse substitutions — from "Substitutions" heading in this part's section
	// Find the substitutions section after this part's data-number
	marker := `data-number='` + partNumber + `'`
	partIdx := strings.Index(html, marker)
	if partIdx < 0 {
		// Try with dashes
		partIdx = strings.Index(html, partNumber)
	}
	if partIdx > 0 {
		subSearch := html[partIdx:]
		// Find next "Substitutions" heading after this part
		subsIdx := strings.Index(subSearch, "Substitutions")
		if subsIdx < 0 {
			subsIdx = strings.Index(subSearch, "substitutions")
		}
		if subsIdx > 0 {
			end := subsIdx + 5000
			if end > len(subSearch) {
				end = len(subSearch)
			}
			subsSection := subSearch[subsIdx:end]
			subMatches := reSubstitution.FindAllStringSubmatch(subsSection, 10)
			subSeen := make(map[string]bool)
			for _, m := range subMatches {
				pn := strings.TrimSpace(m[2])
				if normalizePartNumber(pn) == partNumber || subSeen[pn] {
					continue
				}
				subSeen[pn] = true
				result.Substitutions = append(result.Substitutions, model.SubstitutionPart{
					PartNumber:  pn,
					Description: humanizeDescription(strings.TrimSpace(m[3])),
					Make:        strings.TrimSpace(m[1]),
				})
			}
		}
	}

	// 4. Parse aftermarket alternatives scoped to this part's section
	afterSeen := make(map[string]bool)
	querySection := findProductSection(html, partNumber)
	if querySection != "" {
		afterMatches := reAftermarket.FindAllStringSubmatch(querySection, 20)
		for _, m := range afterMatches {
			brand := strings.TrimSpace(m[1])
			pn := strings.TrimSpace(m[2])
			key := brand + "|" + pn
			if afterSeen[key] {
				continue
			}
			afterSeen[key] = true
			result.Aftermarket = append(result.Aftermarket, model.AftermarketPart{
				PartNumber:  pn,
				Description: strings.TrimSpace(m[3]),
				Brand:       brand,
			})
		}
	}
	// Fallback: scan full page for aftermarket
	if len(result.Aftermarket) == 0 {
		afterMatches := reAftermarket.FindAllStringSubmatch(html, 20)
		for _, m := range afterMatches {
			brand := strings.TrimSpace(m[1])
			pn := strings.TrimSpace(m[2])
			key := brand + "|" + pn
			if afterSeen[key] {
				continue
			}
			afterSeen[key] = true
			result.Aftermarket = append(result.Aftermarket, model.AftermarketPart{
				PartNumber:  pn,
				Description: strings.TrimSpace(m[3]),
				Brand:       brand,
			})
		}
	}

	// 5. Parse compatibility for this specific part
	result.Compatibility = parseCompatibility(html, partNumber)

	return result
}

// parseCompatibility extracts vehicle names from the HTML.
// It checks two sources:
//  1. Inline compatibility divs scoped to the queried part (data-number attribute)
//  2. The "Compatible Models" bottom section
func parseCompatibility(html, queryNumber string) []string {
	seen := make(map[string]bool)
	var vehicles []string

	addVehicle := func(v string) {
		v = strings.TrimSpace(stripTags(v))
		v = strings.Trim(v, "•· \t")
		if len(v) < 2 || seen[v] || v == "..." {
			return
		}
		// Filter out non-vehicle text
		low := strings.ToLower(v)
		if strings.HasPrefix(low, "customer") || strings.HasPrefix(low, "information") ||
			strings.HasPrefix(low, "contact") || strings.HasPrefix(low, "faq") ||
			strings.HasPrefix(low, "about") || strings.HasPrefix(low, "policies") {
			return
		}
		seen[v] = true
		vehicles = append(vehicles, v)
	}

	// Source 1: Find inline compatibility for our exact part number
	// HTML pattern: data-number='58101D3A10'...><ul class='mt-10'><li>Tucson<li>Accent 06</ul>
	pattern := `data-number='` + queryNumber + `'[^>]*>.*?<ul[^>]*>(.*?)</ul>`
	re := regexp.MustCompile(`(?is)` + pattern)
	for _, m := range re.FindAllStringSubmatch(html, 5) {
		for _, li := range reLiContent.FindAllStringSubmatch(m[1], 30) {
			addVehicle(li[1])
		}
	}

	// Source 2: Also grab compatibility from substitution parts (they fit same vehicle)
	if len(vehicles) <= 1 {
		// If queried part only shows e.g. "Tucson" (no year info), also check substitution entries
		reAllCompat := regexp.MustCompile(`(?is)data-number='[^']+?'[^>]*>.*?<ul[^>]*class='mt-10'[^>]*>(.*?)</ul>`)
		for _, m := range reAllCompat.FindAllStringSubmatch(html, 10) {
			for _, li := range reLiContent.FindAllStringSubmatch(m[1], 30) {
				addVehicle(li[1])
			}
		}
	}

	// Source 3: "Compatible Models" bottom section
	// HTML: <h3>Compatible Models</h3><ul><li><strong>Vehicle</strong>...</ul>
	cmIdx := strings.Index(html, "Compatible Models")
	if cmIdx > 0 {
		end := cmIdx + 2000
		if end > len(html) {
			end = len(html)
		}
		section := html[cmIdx:end]
		ulStart := strings.Index(section, "<ul")
		ulEnd := strings.Index(section, "</ul>")
		if ulStart >= 0 && ulEnd > ulStart {
			ulContent := section[ulStart:ulEnd]
			for _, li := range reLiContent.FindAllStringSubmatch(ulContent, 30) {
				addVehicle(li[1])
			}
		}
	}

	return vehicles
}

// findProductSection returns the HTML chunk for the product listing matching the given part number.
// It looks for data-number='partNumber' and returns surrounding context.
func findProductSection(html, partNumber string) string {
	marker := `data-number='` + partNumber + `'`
	idx := strings.Index(html, marker)
	if idx < 0 {
		return ""
	}
	// Go back ~2000 chars and forward ~3000 chars to capture the product section
	start := idx - 2000
	if start < 0 {
		start = 0
	}
	end := idx + 3000
	if end > len(html) {
		end = len(html)
	}
	return html[start:end]
}

// humanizeDescription expands condensed PartsOuq descriptions.
// e.g. "PADKIT-FRONTDISCB" → "PAD KIT - FRONT DISC BRAKE"
func humanizeDescription(desc string) string {
	if !strings.Contains(desc, "-") && strings.Contains(desc, " ") {
		return desc // already has spaces, no condensed parts
	}

	// Common HK abbreviation expansions
	replacer := strings.NewReplacer(
		"PADKIT", "PAD KIT",
		"DISCB", "DISC BRAKE",
		"FRONTDISCB", "FRONT DISC BRAKE",
		"REARDISCB", "REAR DISC BRAKE",
		"FRONTDISC", "FRONT DISC",
		"REARDISC", "REAR DISC",
		"FILTERASSY", "FILTER ASSY",
		"SENSORASSY", "SENSOR ASSY",
		"BRACKETASSY", "BRACKET ASSY",
		"LAMPASSY", "LAMP ASSY",
		"MOTORASSY", "MOTOR ASSY",
		"PUMPASSY", "PUMP ASSY",
		"MANIFOLDASSY", "MANIFOLD ASSY",
		"INJECTORASSY", "INJECTOR ASSY",
		"COMPRESSORASSY", "COMPRESSOR ASSY",
		"COVERASSY", "COVER ASSY",
		"MODULEASSY", "MODULE ASSY",
		"SWITCHASSY", "SWITCH ASSY",
		"HANDLEASSY", "HANDLE ASSY",
		"GRILLE ASSY", "GRILLE ASSY",
		"ENDASSY", "END ASSY",
	)

	// Split on dash, expand each piece, rejoin
	parts := strings.Split(desc, "-")
	for i, p := range parts {
		parts[i] = replacer.Replace(strings.TrimSpace(p))
	}
	return strings.Join(parts, " - ")
}

func stripTags(s string) string {
	return reStripTags.ReplaceAllString(s, "")
}
