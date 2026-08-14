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

// DealerLookup scrapes Hyundai/KIA dealer part sites for OEM part descriptions.
// Used as a fallback when TecDoc + PartsOuq fail.
type DealerLookup struct {
	client    *http.Client
	mu        sync.Mutex
	lastFetch time.Time
	cache     *PartsCache
}

func NewDealerLookup(cache *PartsCache) *DealerLookup {
	return &DealerLookup{
		client: &http.Client{Timeout: 12 * time.Second},
		cache:  cache,
	}
}

// LookupPart tries multiple dealer sites to find a description for an OEM part.
func (d *DealerLookup) LookupPart(partNumber string) *model.OnlinePartResult {
	// Check dealer cache first
	if d.cache != nil {
		if cached := d.cache.GetDealerPart(partNumber); cached != nil {
			return cached
		}
	}

	normalized := normalizePN(partNumber)
	if len(normalized) < 5 {
		return nil
	}

	// Try hyundaipartsdeal.com
	result := d.tryHyundaiPartsDeal(normalized)
	if result != nil {
		result.Make = "Hyundai / KIA"
		if d.cache != nil {
			d.cache.StoreDealerPart(normalized, result.Description, result.Make, result.Category, "hyundaipartsdeal")
		}
		return result
	}

	// Try kiapartsnow.com
	result = d.tryKiaPartsNow(normalized)
	if result != nil {
		result.Make = "Hyundai / KIA"
		if d.cache != nil {
			d.cache.StoreDealerPart(normalized, result.Description, result.Make, result.Category, "kiapartsnow")
		}
		return result
	}

	return nil
}

func (d *DealerLookup) rateLimit() {
	d.mu.Lock()
	elapsed := time.Since(d.lastFetch)
	if elapsed < time.Second {
		d.mu.Unlock()
		time.Sleep(time.Second - elapsed)
		d.mu.Lock()
	}
	d.lastFetch = time.Now()
	d.mu.Unlock()
}

func (d *DealerLookup) fetch(url string) (string, error) {
	d.rateLimit()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := d.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

var (
	hpdTitleRe = regexp.MustCompile(`<h1[^>]*class="[^"]*product[^"]*"[^>]*>([^<]+)</h1>`)
	hpdDescRe  = regexp.MustCompile(`<span[^>]*class="[^"]*product-name[^"]*"[^>]*>([^<]+)</span>`)
	hpdH1Re    = regexp.MustCompile(`<h1[^>]*>([^<]+)</h1>`)
	kpnTitleRe = regexp.MustCompile(`<h1[^>]*>([^<]+)</h1>`)
	kpnDescRe  = regexp.MustCompile(`<div[^>]*class="[^"]*product-name[^"]*"[^>]*>([^<]+)</div>`)
	titleTagRe = regexp.MustCompile(`<title[^>]*>([^<]+)</title>`)
)

func (d *DealerLookup) tryHyundaiPartsDeal(normalized string) *model.OnlinePartResult {
	// Format: https://www.hyundaipartsdeal.com/genuine/hyundai~{part}~{dashed}.html
	dashed := insertDash(normalized)
	url := fmt.Sprintf("https://www.hyundaipartsdeal.com/genuine/hyundai~%s~%s.html",
		strings.ToLower(normalized), strings.ToLower(dashed))

	body, err := d.fetch(url)
	if err != nil {
		log.Printf("dealer hyundai fetch err for %s: %v", normalized, err)
		return nil
	}

	desc := extractDescription(body)
	if desc == "" {
		return nil
	}

	return &model.OnlinePartResult{
		PartNumber:  dashed,
		Description: desc,
		Source:      "dealer_hyundaipartsdeal",
	}
}

func (d *DealerLookup) tryKiaPartsNow(normalized string) *model.OnlinePartResult {
	// Format: https://www.kiapartsnow.com/genuine/kia~{part}~{dashed}.html
	dashed := insertDash(normalized)
	url := fmt.Sprintf("https://www.kiapartsnow.com/genuine/kia~%s~%s.html",
		strings.ToLower(normalized), strings.ToLower(dashed))

	body, err := d.fetch(url)
	if err != nil {
		log.Printf("dealer kia fetch err for %s: %v", normalized, err)
		return nil
	}

	desc := extractDescription(body)
	if desc == "" {
		return nil
	}

	return &model.OnlinePartResult{
		PartNumber:  dashed,
		Description: desc,
		Source:      "dealer_kiapartsnow",
	}
}

func isGenericDescription(desc string) bool {
	lower := strings.ToLower(strings.TrimSpace(desc))
	generics := []string{
		"hyundai parts", "kia parts", "genuine hyundai", "genuine kia",
		"oem parts", "auto parts", "car parts", "parts deal",
		"hyundai parts deal", "kia parts now", "page not found",
		"hyundaipartsdeal", "kiapartsnow",
		"genuine oem hyundai parts and accessories online",
		"genuine oem kia parts and accessories online",
		"genuine oem hyundai parts", "genuine oem kia parts",
	}
	for _, g := range generics {
		if lower == g || strings.TrimRight(lower, ".") == g {
			return true
		}
	}
	// Also reject if it contains "parts and accessories" or "online store"
	if strings.Contains(lower, "parts and accessories") || strings.Contains(lower, "online store") {
		return true
	}
	return false
}

func extractDescription(html string) string {
	// Try various title/description patterns
	for _, re := range []*regexp.Regexp{hpdTitleRe, hpdDescRe, hpdH1Re, kpnTitleRe, kpnDescRe} {
		if m := re.FindStringSubmatch(html); len(m) > 1 {
			desc := strings.TrimSpace(m[1])
			// Filter out generic/useless titles
			lower := strings.ToLower(desc)
			if strings.Contains(lower, "404") || strings.Contains(lower, "not found") ||
				strings.Contains(lower, "error") || len(desc) < 3 || len(desc) > 200 {
				continue
			}
			cleaned := cleanDescription(desc)
			if isGenericDescription(cleaned) {
				continue
			}
			return cleaned
		}
	}

	// Fallback: try <title> tag
	if m := titleTagRe.FindStringSubmatch(html); len(m) > 1 {
		desc := strings.TrimSpace(m[1])
		lower := strings.ToLower(desc)
		if !strings.Contains(lower, "404") && !strings.Contains(lower, "not found") &&
			len(desc) >= 5 && len(desc) <= 200 {
			// Clean up title tag (often has site name appended)
			if idx := strings.Index(desc, " - "); idx > 0 {
				desc = desc[:idx]
			}
			if idx := strings.Index(desc, " | "); idx > 0 {
				desc = desc[:idx]
			}
			cleaned := cleanDescription(desc)
			if !isGenericDescription(cleaned) {
				return cleaned
			}
		}
	}

	return ""
}

func cleanDescription(desc string) string {
	// Remove leading part numbers like "26300-35503 " or "26300355033 "
	desc = strings.TrimSpace(desc)
	if len(desc) > 12 {
		// Check if first word looks like a part number
		parts := strings.SplitN(desc, " ", 2)
		if len(parts) == 2 && looksLikePartNum(parts[0]) {
			desc = strings.TrimSpace(parts[1])
		}
	}
	// Strip "Genuine Hyundai/KIA" prefix from dealer sites
	for _, prefix := range []string{
		"genuine hyundai ", "genuine kia ", "oem hyundai ", "oem kia ",
		"hyundai genuine ", "kia genuine ",
	} {
		lower := strings.ToLower(desc)
		if strings.HasPrefix(lower, prefix) {
			desc = strings.TrimSpace(desc[len(prefix):])
		}
	}
	return strings.ToUpper(strings.TrimSpace(desc))
}

func looksLikePartNum(s string) bool {
	digits := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			digits++
		}
	}
	return digits >= 5
}

func insertDash(norm string) string {
	if len(norm) >= 10 {
		return norm[:5] + "-" + norm[5:]
	}
	if len(norm) >= 8 {
		return norm[:5] + "-" + norm[5:]
	}
	return norm
}

func normalizePN(pn string) string {
	var b strings.Builder
	for _, c := range strings.ToUpper(pn) {
		if (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') {
			b.WriteRune(c)
		}
	}
	return b.String()
}
