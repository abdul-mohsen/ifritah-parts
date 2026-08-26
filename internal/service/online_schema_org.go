package service

// M8.T6 — Schema.org JSON-LD extractor.
//
// Many public reference sites (HyundaiPartsDeal.com, KiaPartsNow.com,
// PartsGeek.com, AutoZone.com, etc.) publish product data as JSON-LD
// blocks inside `<script type="application/ld+json">` tags — this is
// intended for machine consumption by Google, Bing, and structured-
// data crawlers. Extracting it is standard SEO practice and does not
// require scraping the visible HTML.
//
// This parser supports the subset of schema.org relevant to
// automotive parts:
//
//   Product     — brand, name, mpn, sku, image, offers
//   Offer       — price, priceCurrency, itemCondition, url
//   Brand       — string or nested {"@type":"Brand", "name": "..."}
//   ItemList    — collection of Products (used by search-result pages)
//
// The extractor is deliberately DEFENSIVE: sites vary in shape (some
// wrap in `@graph`, some use `Offer` at top level, some use `AggregateOffer`).
// We accept several shapes and skip anything we don't understand rather
// than fail the whole parse.

import (
	"encoding/json"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"parts-engine/internal/model"
)

// ExtractSchemaOrgProducts pulls every Product entry from HTML and
// returns them as AftermarketPart records. `sourceName` and
// `sourceURL` are stamped onto every returned row. Trust score is NOT
// set here — the adapter that calls this function applies its own
// trust tier.
//
// Empty input, no JSON-LD blocks, or unparseable blocks all return
// (nil, nil) — the caller treats absence as "no results" rather than
// error.
func ExtractSchemaOrgProducts(htmlBody []byte, sourceName, sourceURL string) ([]model.AftermarketPart, error) {
	if len(htmlBody) == 0 {
		return nil, nil
	}

	blocks, err := extractLdBlocks(htmlBody)
	if err != nil {
		return nil, err
	}
	if len(blocks) == 0 {
		return nil, nil
	}

	var out []model.AftermarketPart
	for _, block := range blocks {
		out = append(out, walkLdNode(block, sourceName, sourceURL)...)
	}
	return out, nil
}

// extractLdBlocks walks the HTML tree and returns the raw JSON bytes of
// every `<script type="application/ld+json">` block.
func extractLdBlocks(body []byte) ([][]byte, error) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	var blocks [][]byte
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "script" {
			for _, a := range n.Attr {
				if strings.EqualFold(a.Key, "type") && strings.EqualFold(a.Val, "application/ld+json") {
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						if c.Type == html.TextNode && strings.TrimSpace(c.Data) != "" {
							blocks = append(blocks, []byte(c.Data))
						}
					}
					break
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return blocks, nil
}

// walkLdNode parses a single JSON-LD block and returns any Product
// entries as AftermarketPart records. Recursively descends into
// `@graph`, arrays, and `ItemList.itemListElement`.
func walkLdNode(raw []byte, sourceName, sourceURL string) []model.AftermarketPart {
	// Try array-of-nodes first, then single object.
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil
	}
	if trimmed[0] == '[' {
		var arr []json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &arr); err != nil {
			return nil
		}
		var out []model.AftermarketPart
		for _, elem := range arr {
			out = append(out, walkLdNode(elem, sourceName, sourceURL)...)
		}
		return out
	}

	var node map[string]json.RawMessage
	if err := json.Unmarshal(raw, &node); err != nil {
		return nil
	}

	// Inspect @type.
	types := extractTypeSet(node)
	var out []model.AftermarketPart

	// Direct Product node.
	if types["Product"] {
		if p, ok := productFromNode(node, sourceName, sourceURL); ok {
			out = append(out, p)
		}
	}

	// @graph: array of nested nodes (JSON-LD 1.1 canonical form)
	if graphRaw, has := node["@graph"]; has {
		out = append(out, walkLdNode(graphRaw, sourceName, sourceURL)...)
	}

	// ItemList.itemListElement — search-result pages
	if types["ItemList"] {
		if itemsRaw, has := node["itemListElement"]; has {
			out = append(out, walkLdNode(itemsRaw, sourceName, sourceURL)...)
		}
	}

	// ListItem.item — inner wrapper commonly used inside ItemList
	if types["ListItem"] {
		if inner, has := node["item"]; has {
			out = append(out, walkLdNode(inner, sourceName, sourceURL)...)
		}
	}

	return out
}

// productFromNode extracts an AftermarketPart from a schema.org
// Product node. Returns (_, false) if brand or partNumber can't be
// resolved.
func productFromNode(node map[string]json.RawMessage, sourceName, sourceURL string) (model.AftermarketPart, bool) {
	name := stringField(node, "name")
	brand := extractBrandNode(node["brand"])
	mpn := stringField(node, "mpn")
	sku := stringField(node, "sku")
	image := stringField(node, "image")
	url := stringField(node, "url")

	// Prefer MPN, fall back to SKU as the part-number identifier.
	partNumber := mpn
	if partNumber == "" {
		partNumber = sku
	}
	if partNumber == "" || brand == "" {
		return model.AftermarketPart{}, false
	}

	// Effective source URL: prefer per-product url, fall back to the page URL.
	effURL := url
	if effURL == "" {
		effURL = sourceURL
	}

	priceCents, currency, condition := extractOfferFields(node)

	part := model.AftermarketPart{
		PartNumber:  partNumber,
		Description: name,
		Brand:       brand,
		Source:      sourceName,
		SourceURL:   effURL,
		PriceCents:  priceCents,
		Currency:    currency,
		Condition:   condition,
		ImageURL:    image,
	}
	return part, true
}

// extractTypeSet reads the "@type" field which may be a string or an
// array of strings.
func extractTypeSet(node map[string]json.RawMessage) map[string]bool {
	raw, ok := node["@type"]
	if !ok {
		return map[string]bool{}
	}
	set := map[string]bool{}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil && s != "" {
		set[s] = true
		return set
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		for _, t := range arr {
			set[t] = true
		}
	}
	return set
}

// stringField returns a leaf string; tolerates single-element arrays
// which some schema.org publishers use.
func stringField(node map[string]json.RawMessage, key string) string {
	raw, ok := node[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) > 0 {
		return arr[0]
	}
	return ""
}

// extractBrandNode resolves the "brand" field which may be:
//   - a plain string: "BOSCH"
//   - an object: {"@type":"Brand", "name":"BOSCH"}
//   - an array of either
func extractBrandNode(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil && s != "" {
		return s
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err == nil {
		if name := stringField(obj, "name"); name != "" {
			return name
		}
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		for _, elem := range arr {
			if b := extractBrandNode(elem); b != "" {
				return b
			}
		}
	}
	return ""
}

// extractOfferFields pulls price + currency + condition out of the
// "offers" field. Supports both direct Offer objects and AggregateOffer
// wrappers. Missing fields return zero values.
func extractOfferFields(node map[string]json.RawMessage) (priceCents int64, currency, condition string) {
	raw, ok := node["offers"]
	if !ok {
		return 0, "", ""
	}
	// Try single Offer object first.
	if obj, ok := parseObjectOrFirstOfArray(raw); ok {
		if agg := stringField(obj, "@type"); strings.Contains(agg, "AggregateOffer") {
			// Recurse into "lowPrice" or "offers" child if present.
			priceCents = parsePriceToCents(obj["lowPrice"])
			currency = stringField(obj, "priceCurrency")
			if inner, has := obj["offers"]; has {
				if innerObj, ok := parseObjectOrFirstOfArray(inner); ok {
					if p := parsePriceToCents(innerObj["price"]); p > 0 {
						priceCents = p
					}
					if c := stringField(innerObj, "priceCurrency"); c != "" {
						currency = c
					}
					condition = canonicaliseSchemaOrgCondition(stringField(innerObj, "itemCondition"))
				}
			}
			return
		}
		priceCents = parsePriceToCents(obj["price"])
		currency = stringField(obj, "priceCurrency")
		condition = canonicaliseSchemaOrgCondition(stringField(obj, "itemCondition"))
	}
	return
}

// parseObjectOrFirstOfArray tolerates offers being an object or a
// 1-element array of objects.
func parseObjectOrFirstOfArray(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err == nil {
		return obj, true
	}
	var arr []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) > 0 {
		return arr[0], true
	}
	return nil, false
}

// parsePriceToCents accepts price as string or number and converts to
// integer cents.
func parsePriceToCents(raw json.RawMessage) int64 {
	if len(raw) == 0 {
		return 0
	}
	// Try number first.
	var n float64
	if err := json.Unmarshal(raw, &n); err == nil {
		if n < 0 {
			return 0
		}
		return int64(n*100 + 0.5)
	}
	// Try string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		s = strings.TrimSpace(s)
		// Strip common currency symbols
		s = strings.TrimPrefix(s, "$")
		s = strings.TrimPrefix(s, "£")
		s = strings.TrimPrefix(s, "€")
		s = strings.TrimSpace(s)
		s = strings.ReplaceAll(s, ",", "")
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || f < 0 {
			return 0
		}
		return int64(f*100 + 0.5)
	}
	return 0
}

// canonicaliseSchemaOrgCondition maps schema.org condition URIs to our
// small enumeration.
func canonicaliseSchemaOrgCondition(raw string) string {
	if raw == "" {
		return "unknown"
	}
	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "newcondition"), strings.Contains(lower, "new"):
		return "new"
	case strings.Contains(lower, "refurbishedcondition"), strings.Contains(lower, "refurb"), strings.Contains(lower, "reman"):
		return "reman"
	case strings.Contains(lower, "usedcondition"), strings.Contains(lower, "used"):
		return "used"
	case strings.Contains(lower, "damagedcondition"):
		return "used"
	default:
		return "unknown"
	}
}
