package service

// M8.T2 — eBay Motors Finding API adapter.
//
// Uses eBay's official free Finding Service (5,000 calls/day free tier)
// to look up parts by OEM number. The Finding Service is a legacy REST
// API that predates the modern eBay Buy API but requires no OAuth — a
// simple app-ID query parameter authenticates the caller. Perfect for
// server-side inclusion in a UNION path.
//
// Auth: the app-ID is read from EBAY_APP_ID. When unset, the adapter's
// Enabled() returns false and Search() returns (nil, nil) — no error.
// This is by design so a missing secret disables the source without
// requiring a redeploy.
//
// Request shape (URL-encoded query string):
//
//	GET https://svcs.ebay.com/services/search/FindingService/v1
//	   ?OPERATION-NAME=findItemsAdvanced
//	   &SERVICE-VERSION=1.0.0
//	   &SECURITY-APPNAME=<EBAY_APP_ID>
//	   &RESPONSE-DATA-FORMAT=JSON
//	   &keywords=<OEM>+Hyundai
//	   &categoryId=6028              // Motors > Parts & Accessories
//	   &itemFilter(0).name=Condition
//	   &itemFilter(0).value=New
//	   &paginationInput.entriesPerPage=25
//
// Response: nested JSON. We extract only the fields we need:
//   title, itemId, viewItemURL, sellingStatus.currentPrice.__value__,
//   sellingStatus.currentPrice.@currencyId, condition.conditionDisplayName,
//   productId (for MPN) when present.
//
// Brand extraction: eBay's Finding API does not expose itemSpecifics in
// the same shape as the Buy API. We fall back to a regex match against
// NormalizeBrand's canonical set over the item title. If no brand can be
// determined, the result is dropped (Brand="" fails the UNION dedupe
// downstream anyway).

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"parts-engine/internal/model"
)

const (
	ebaySourceName    = "online:ebay"
	ebayEndpoint      = "https://svcs.ebay.com/services/search/FindingService/v1"
	ebayHTTPTimeout   = 5 * time.Second
	ebayRateInterval  = 300 * time.Millisecond // ~3 req/sec, well under any tier
	ebayEntriesPerReq = 25
)

// EbayFinder is the OnlineSource adapter for eBay Motors.
type EbayFinder struct {
	appID  string
	client *http.Client
}

// NewEbayFinder returns an EbayFinder wired to the app-ID in EBAY_APP_ID.
// Pass nil for client to use a shared default with a 5s per-request
// timeout.
func NewEbayFinder(client *http.Client) *EbayFinder {
	if client == nil {
		client = &http.Client{Timeout: ebayHTTPTimeout}
	}
	return &EbayFinder{
		appID:  strings.TrimSpace(os.Getenv("EBAY_APP_ID")),
		client: client,
	}
}

// Name identifies this source in provenance tags.
func (f *EbayFinder) Name() string { return ebaySourceName }

// Enabled returns true only when EBAY_APP_ID is set AND
// ONLINE_EBAY_ENABLED is not explicitly set to "false".
func (f *EbayFinder) Enabled() bool {
	if f == nil || f.appID == "" {
		return false
	}
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("ONLINE_EBAY_ENABLED"))); v == "false" || v == "0" || v == "no" {
		return false
	}
	return true
}

// RateLimit returns the minimum interval between outbound eBay calls.
func (f *EbayFinder) RateLimit() time.Duration { return ebayRateInterval }

// TrustScore returns the trust tier for eBay Motors results. eBay item
// titles are seller-provided so brand attribution is noisier than
// brand-direct or dealer sources — we use the marketplace-noisy tier
// to demote eBay results below authoritative sources when the same
// part appears in multiple.
func (f *EbayFinder) TrustScore() float64 { return TrustMarketplaceNoisy }

// Search queries the Finding API for the OEM and returns aftermarket
// parts with brand + part-number + price + click-through URL populated.
// Returns (nil, nil) if the source is disabled or the OEM is empty.
func (f *EbayFinder) Search(ctx context.Context, oemNormalized string) ([]model.AftermarketPart, error) {
	if !f.Enabled() {
		return nil, nil
	}
	if oemNormalized == "" {
		return nil, nil
	}

	// eBay expects the OEM as a keyword; append "Hyundai OR Kia" so the
	// listings we care about float to the top of the relevance ranking.
	keywords := fmt.Sprintf("%s Hyundai Kia", oemNormalized)

	q := url.Values{}
	q.Set("OPERATION-NAME", "findItemsAdvanced")
	q.Set("SERVICE-VERSION", "1.0.0")
	q.Set("SECURITY-APPNAME", f.appID)
	q.Set("RESPONSE-DATA-FORMAT", "JSON")
	q.Set("keywords", keywords)
	q.Set("categoryId", "6028") // Motors > Parts & Accessories
	q.Set("paginationInput.entriesPerPage", strconv.Itoa(ebayEntriesPerReq))
	q.Set("itemFilter(0).name", "Condition")
	q.Set("itemFilter(0).value", "New")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ebayEndpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("ebay: build request: %w", err)
	}
	req.Header.Set("User-Agent", robotsClientAgent) // honest UA
	req.Header.Set("Accept", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ebay: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ebay: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // 4 MB cap
	if err != nil {
		return nil, fmt.Errorf("ebay: read body: %w", err)
	}

	return parseEbayFindingResponse(body, oemNormalized)
}

// parseEbayFindingResponse extracts model.AftermarketPart list from the
// (deeply nested, historically weird) eBay Finding API JSON envelope.
//
// The envelope shape is:
//
//	{"findItemsAdvancedResponse":[
//	  {"searchResult":[
//	    {"item":[{ ...item fields... }, ...]}
//	  ], ...}
//	]}
//
// Every leaf value is wrapped in a 1-element array because the eBay
// SOAP-to-JSON translator preserves XML repetition. We tolerate both
// shapes (array of 1 or scalar) via defensive helpers.
func parseEbayFindingResponse(body []byte, oemNormalized string) ([]model.AftermarketPart, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("ebay: parse envelope: %w", err)
	}
	rawResp, ok := envelope["findItemsAdvancedResponse"]
	if !ok {
		return nil, nil // response empty / malformed — treat as no results
	}

	var respArr []map[string]json.RawMessage
	if err := json.Unmarshal(rawResp, &respArr); err != nil || len(respArr) == 0 {
		return nil, nil
	}
	rawSearchResult, ok := respArr[0]["searchResult"]
	if !ok {
		return nil, nil
	}

	var srArr []map[string]json.RawMessage
	if err := json.Unmarshal(rawSearchResult, &srArr); err != nil || len(srArr) == 0 {
		return nil, nil
	}
	rawItems, ok := srArr[0]["item"]
	if !ok {
		return nil, nil
	}

	var items []map[string]json.RawMessage
	if err := json.Unmarshal(rawItems, &items); err != nil {
		return nil, nil
	}

	out := make([]model.AftermarketPart, 0, len(items))
	for _, it := range items {
		part := ebayItemToPart(it, oemNormalized)
		if part.PartNumber == "" || part.Brand == "" {
			continue // skip rows we can't attribute confidently
		}
		out = append(out, part)
	}
	return out, nil
}

// ebayItemToPart converts one eBay item entry to model.AftermarketPart.
func ebayItemToPart(item map[string]json.RawMessage, oemNormalized string) model.AftermarketPart {
	title := firstString(item, "title")
	viewURL := firstString(item, "viewItemURL")
	gallery := firstString(item, "galleryURL")

	// Brand: try Finding-API's productId (rare) → title regex over
	// NormalizeBrand's canonical set → give up.
	brand := extractBrandFromTitle(title)

	// PartNumber: use the OEM the caller queried as the identity anchor.
	// eBay's Finding API doesn't reliably surface MPN, so we tag the row
	// with the OEM itself; the UNION dedupe key is (brand, part_number)
	// so multiple eBay listings for the same brand+OEM collapse to one
	// row.
	partNumber := oemNormalized

	priceCents, currency := parsePriceFromItem(item)
	condition := firstString(item, "condition", "conditionDisplayName")

	return model.AftermarketPart{
		PartNumber:  partNumber,
		Description: title,
		Brand:       brand,
		Source:      ebaySourceName,
		SourceURL:   viewURL,
		PriceCents:  priceCents,
		Currency:    currency,
		Condition:   canonicaliseCondition(condition),
		ImageURL:    gallery,
	}
}

// firstString extracts a leaf string value from an eBay wrapped-array
// field. Supports both `"foo":["bar"]` and `"foo":"bar"` and nested
// `"foo":[{"bar":["baz"]}]` when `path` has ≥ 2 elements.
func firstString(node map[string]json.RawMessage, path ...string) string {
	if len(path) == 0 {
		return ""
	}
	raw, ok := node[path[0]]
	if !ok {
		return ""
	}
	// Try string first.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if len(path) == 1 {
			return s
		}
		return ""
	}
	// Try array of strings.
	if len(path) == 1 {
		var arr []string
		if err := json.Unmarshal(raw, &arr); err == nil && len(arr) > 0 {
			return arr[0]
		}
	}
	// Try array of objects.
	var arrObj []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &arrObj); err == nil && len(arrObj) > 0 {
		return firstString(arrObj[0], path[1:]...)
	}
	return ""
}

// parsePriceFromItem digs into item.sellingStatus[0].currentPrice[0].__value__ and @currencyId.
func parsePriceFromItem(item map[string]json.RawMessage) (int64, string) {
	rawSS, ok := item["sellingStatus"]
	if !ok {
		return 0, ""
	}
	var ssArr []map[string]json.RawMessage
	if err := json.Unmarshal(rawSS, &ssArr); err != nil || len(ssArr) == 0 {
		return 0, ""
	}
	rawPrice, ok := ssArr[0]["currentPrice"]
	if !ok {
		return 0, ""
	}
	var priceArr []map[string]json.RawMessage
	if err := json.Unmarshal(rawPrice, &priceArr); err != nil || len(priceArr) == 0 {
		return 0, ""
	}
	priceObj := priceArr[0]

	// eBay serialises the numeric value under "__value__" (string) and
	// the currency under "@currencyId" (string).
	var valueStr string
	if raw, ok := priceObj["__value__"]; ok {
		_ = json.Unmarshal(raw, &valueStr)
	}
	var currency string
	if raw, ok := priceObj["@currencyId"]; ok {
		_ = json.Unmarshal(raw, &currency)
	}
	if valueStr == "" {
		return 0, currency
	}
	// Parse as float, convert to cents.
	f, err := strconv.ParseFloat(valueStr, 64)
	if err != nil || f < 0 {
		return 0, currency
	}
	return int64(f*100 + 0.5), currency
}

// extractBrandFromTitle returns the canonical brand name (via
// NormalizeBrand) if any of the canonical brand tokens is present in
// the item title (case-insensitive). Empty string when no brand match.
func extractBrandFromTitle(title string) string {
	if title == "" {
		return ""
	}
	lower := strings.ToLower(title)
	for _, canonical := range ebayCandidateBrands {
		if strings.Contains(lower, strings.ToLower(canonical)) {
			return NormalizeBrand(canonical)
		}
	}
	return ""
}

// ebayCandidateBrands is the short-list of aftermarket brand names we
// try to match in eBay listing titles. Kept in one place so the M2.S2
// canonical-brand map (used by NormalizeBrand) stays the source of truth.
//
// Ordering matters: most-specific first so "MANN-FILTER" wins over
// "MANN" when both appear.
var ebayCandidateBrands = []string{
	"MANN-FILTER", "MANN FILTER", "MANN",
	"BOSCH",
	"MAHLE ORIGINAL", "MAHLE",
	"DENSO",
	"NGK",
	"VALEO",
	"HELLA",
	"BREMBO",
	"TEXTAR",
	"FERODO",
	"SACHS",
	"FEBI BILSTEIN", "FEBI",
	"LEMFOERDER", "LEMFÖRDER",
	"LUK",
	"INA",
	"SKF",
	"GATES",
	"CONTINENTAL",
	"KNECHT",
	"FILTRON",
	"WIX",
	"PURFLUX",
	"MAGNETI MARELLI",
	"DELPHI",
	"MEYLE",
	"TRW",
	"ATE",
	"ZIMMERMANN",
	"BLUE PRINT",
	"KYB",
	"MONROE",
	"BILSTEIN",
	"KONI",
	"CHAMPION",
	"MOBIS",
	"HYUNDAI",
	"KIA",
	"GENUINE",
}

// canonicaliseCondition maps eBay's free-text conditionDisplayName to the
// small enumeration our UI understands.
func canonicaliseCondition(c string) string {
	lower := strings.ToLower(strings.TrimSpace(c))
	switch {
	case lower == "" || lower == "not specified":
		return "unknown"
	case strings.Contains(lower, "new"):
		return "new"
	case strings.Contains(lower, "refurbish") || strings.Contains(lower, "reman") || strings.Contains(lower, "rebuilt"):
		return "reman"
	case strings.Contains(lower, "used") || strings.Contains(lower, "pre-owned") || strings.Contains(lower, "for parts"):
		return "used"
	default:
		return "unknown"
	}
}
