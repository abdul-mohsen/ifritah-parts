package service

import (
	"strings"
	"testing"
)

func TestExtractSchemaOrgProducts_EmptyBody(t *testing.T) {
	parts, err := ExtractSchemaOrgProducts(nil, "test", "https://example.com")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(parts) != 0 {
		t.Errorf("expected 0 parts for nil body, got %d", len(parts))
	}
}

func TestExtractSchemaOrgProducts_NoLdBlocks(t *testing.T) {
	html := `<html><body><h1>No structured data here</h1></body></html>`
	parts, err := ExtractSchemaOrgProducts([]byte(html), "test", "https://example.com")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(parts) != 0 {
		t.Errorf("expected 0 parts, got %d", len(parts))
	}
}

func TestExtractSchemaOrgProducts_SimpleProduct(t *testing.T) {
	// Realistic shape from a dealer-catalog product page: single Product
	// with nested Brand object + Offer wrapper.
	fixture := `<html><head>
<script type="application/ld+json">
{
  "@context": "https://schema.org",
  "@type": "Product",
  "name": "Oil Filter Assembly for 2016-2019 Hyundai Sonata 2.4L",
  "mpn": "26350-2G000",
  "sku": "HDP-26350-2G000",
  "image": "https://cdn.example.com/products/26350-2G000.jpg",
  "url": "https://example.com/products/hyundai/26350-2G000",
  "brand": {"@type": "Brand", "name": "Hyundai Genuine"},
  "offers": {
    "@type": "Offer",
    "price": "12.49",
    "priceCurrency": "USD",
    "itemCondition": "https://schema.org/NewCondition",
    "availability": "https://schema.org/InStock"
  }
}
</script>
</head><body></body></html>`

	parts, err := ExtractSchemaOrgProducts([]byte(fixture), "online:hyundaipartsdeal", "https://example.com/search?q=26350-2G000")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}
	p := parts[0]
	if p.PartNumber != "26350-2G000" {
		t.Errorf("PartNumber = %q, want 26350-2G000", p.PartNumber)
	}
	if !strings.Contains(strings.ToLower(p.Brand), "hyundai") {
		t.Errorf("Brand = %q, want to contain 'hyundai'", p.Brand)
	}
	if p.PriceCents != 1249 {
		t.Errorf("PriceCents = %d, want 1249", p.PriceCents)
	}
	if p.Currency != "USD" {
		t.Errorf("Currency = %q, want USD", p.Currency)
	}
	if p.Condition != "new" {
		t.Errorf("Condition = %q, want new", p.Condition)
	}
	if p.Source != "online:hyundaipartsdeal" {
		t.Errorf("Source = %q, want online:hyundaipartsdeal", p.Source)
	}
	if p.SourceURL != "https://example.com/products/hyundai/26350-2G000" {
		t.Errorf("SourceURL = %q, want product page URL", p.SourceURL)
	}
	if p.ImageURL == "" {
		t.Errorf("ImageURL should be set")
	}
}

func TestExtractSchemaOrgProducts_ItemListPage(t *testing.T) {
	// Search-results page: ItemList wrapping multiple products via ListItem.
	fixture := `<html><head>
<script type="application/ld+json">
{
  "@context": "https://schema.org",
  "@type": "ItemList",
  "itemListElement": [
    {
      "@type": "ListItem",
      "position": 1,
      "item": {
        "@type": "Product",
        "name": "BOSCH Oil Filter for Hyundai",
        "mpn": "F026407008",
        "brand": "BOSCH",
        "offers": {"@type": "Offer", "price": "8.99", "priceCurrency": "USD"}
      }
    },
    {
      "@type": "ListItem",
      "position": 2,
      "item": {
        "@type": "Product",
        "name": "MANN-FILTER Oil Filter for Kia",
        "mpn": "W811-80",
        "brand": {"@type": "Brand", "name": "MANN-FILTER"},
        "offers": {"@type": "Offer", "price": "10.49", "priceCurrency": "EUR"}
      }
    }
  ]
}
</script>
</head><body></body></html>`

	parts, err := ExtractSchemaOrgProducts([]byte(fixture), "online:partsgeek", "https://example.com/search")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d: %+v", len(parts), parts)
	}
	// Verify both extractions
	var bosch, mann bool
	for _, p := range parts {
		if p.PartNumber == "F026407008" && strings.EqualFold(p.Brand, "BOSCH") {
			bosch = true
			if p.Currency != "USD" || p.PriceCents != 899 {
				t.Errorf("Bosch: got price=%d currency=%q", p.PriceCents, p.Currency)
			}
		}
		if p.PartNumber == "W811-80" && strings.Contains(strings.ToUpper(p.Brand), "MANN") {
			mann = true
		}
	}
	if !bosch {
		t.Errorf("BOSCH product not extracted")
	}
	if !mann {
		t.Errorf("MANN-FILTER product not extracted")
	}
}

func TestExtractSchemaOrgProducts_GraphContainer(t *testing.T) {
	// JSON-LD 1.1 @graph shape.
	fixture := `<html><head>
<script type="application/ld+json">
{
  "@context": "https://schema.org",
  "@graph": [
    {
      "@type": "WebPage",
      "name": "Search results"
    },
    {
      "@type": "Product",
      "name": "HELLA Cabin Filter",
      "mpn": "8FH-351-000-514",
      "brand": "HELLA",
      "offers": {"@type": "Offer", "price": "$18.99", "priceCurrency": "USD"}
    }
  ]
}
</script>
</head><body></body></html>`

	parts, err := ExtractSchemaOrgProducts([]byte(fixture), "online:carid", "https://example.com/p")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(parts) != 1 {
		t.Fatalf("expected 1 product, got %d", len(parts))
	}
	if parts[0].PartNumber != "8FH-351-000-514" {
		t.Errorf("PartNumber = %q", parts[0].PartNumber)
	}
	// Price parsing with $ prefix should work
	if parts[0].PriceCents != 1899 {
		t.Errorf("PriceCents = %d, want 1899 (parsed from '$18.99')", parts[0].PriceCents)
	}
}

func TestExtractSchemaOrgProducts_MissingBrandDropsRow(t *testing.T) {
	// Product without brand — should be dropped (can't attribute).
	fixture := `<html><head>
<script type="application/ld+json">
{
  "@type": "Product",
  "name": "Some part",
  "mpn": "12345"
}
</script>
</head></html>`

	parts, err := ExtractSchemaOrgProducts([]byte(fixture), "test", "https://x.y")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(parts) != 0 {
		t.Errorf("expected 0 parts (no brand), got %d", len(parts))
	}
}

func TestExtractSchemaOrgProducts_MissingMPNAndSKUDropsRow(t *testing.T) {
	fixture := `<html><head>
<script type="application/ld+json">
{
  "@type": "Product",
  "name": "Some part",
  "brand": "BOSCH"
}
</script>
</head></html>`

	parts, err := ExtractSchemaOrgProducts([]byte(fixture), "test", "https://x.y")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(parts) != 0 {
		t.Errorf("expected 0 parts (no mpn/sku), got %d", len(parts))
	}
}

func TestExtractSchemaOrgProducts_AggregateOffer(t *testing.T) {
	// Some sites use AggregateOffer wrapping the actual Offer.
	fixture := `<html><head>
<script type="application/ld+json">
{
  "@type": "Product",
  "name": "Bosch Air Filter",
  "mpn": "S3948",
  "brand": "Bosch",
  "offers": {
    "@type": "AggregateOffer",
    "lowPrice": 15.99,
    "highPrice": 22.99,
    "priceCurrency": "USD",
    "offerCount": 3,
    "offers": [{
      "@type": "Offer",
      "price": "16.49",
      "priceCurrency": "USD",
      "itemCondition": "https://schema.org/NewCondition"
    }]
  }
}
</script>
</head></html>`

	parts, err := ExtractSchemaOrgProducts([]byte(fixture), "test", "https://x.y")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}
	// Should prefer the inner Offer's price (16.49) over lowPrice (15.99)
	if parts[0].PriceCents != 1649 {
		t.Errorf("PriceCents = %d, want 1649 (from inner Offer)", parts[0].PriceCents)
	}
	if parts[0].Condition != "new" {
		t.Errorf("Condition = %q, want new", parts[0].Condition)
	}
}

func TestExtractSchemaOrgProducts_MalformedJSONSkipped(t *testing.T) {
	fixture := `<html><head>
<script type="application/ld+json">this is not json</script>
<script type="application/ld+json">
{"@type":"Product","name":"Real Product","mpn":"REAL-1","brand":"BOSCH","offers":{"@type":"Offer","price":"9.99","priceCurrency":"USD"}}
</script>
</head></html>`

	parts, err := ExtractSchemaOrgProducts([]byte(fixture), "test", "https://x.y")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(parts) != 1 {
		t.Fatalf("expected 1 part (garbage block skipped), got %d", len(parts))
	}
	if parts[0].PartNumber != "REAL-1" {
		t.Errorf("PartNumber = %q", parts[0].PartNumber)
	}
}
