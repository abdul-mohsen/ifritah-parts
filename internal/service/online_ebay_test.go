package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// TestEbayFinder_DisabledWhenNoAppID verifies the adapter is inert when
// EBAY_APP_ID is unset — Enabled() returns false and Search() returns
// (nil, nil) with no HTTP call.
func TestEbayFinder_DisabledWhenNoAppID(t *testing.T) {
	os.Unsetenv("EBAY_APP_ID")
	f := NewEbayFinder(nil)
	if f.Enabled() {
		t.Fatalf("Enabled() = true with no EBAY_APP_ID")
	}
	parts, err := f.Search(context.Background(), "263202G000")
	if err != nil {
		t.Fatalf("Search err: %v", err)
	}
	if len(parts) != 0 {
		t.Fatalf("expected 0 parts when disabled, got %d", len(parts))
	}
}

// TestEbayFinder_DisabledWhenFlagFalse verifies the ONLINE_EBAY_ENABLED
// kill switch works even with an app-ID present.
func TestEbayFinder_DisabledWhenFlagFalse(t *testing.T) {
	t.Setenv("EBAY_APP_ID", "test-app-id")
	t.Setenv("ONLINE_EBAY_ENABLED", "false")
	f := NewEbayFinder(nil)
	if f.Enabled() {
		t.Errorf("expected Enabled=false when ONLINE_EBAY_ENABLED=false")
	}
}

// TestEbayFinder_ParsesRealisticResponse covers the whole envelope-parse
// pipeline against a fixture that mirrors the shape eBay actually
// returns.
func TestEbayFinder_ParsesRealisticResponse(t *testing.T) {
	fixture := ebayFixtureBoschOilFilter()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fixture))
	}))
	defer srv.Close()

	// The finder's endpoint is hard-coded but we can intercept via a
	// custom RoundTripper that redirects to our httptest server.
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &redirectingTransport{target: srv.URL},
	}
	t.Setenv("EBAY_APP_ID", "test-app-id")
	t.Setenv("ONLINE_EBAY_ENABLED", "true")
	f := NewEbayFinder(client)

	parts, err := f.Search(context.Background(), "263202G000")
	if err != nil {
		t.Fatalf("Search err: %v", err)
	}
	if len(parts) == 0 {
		t.Fatalf("expected results, got 0")
	}

	// The fixture has one Bosch listing. Confirm the parse.
	var found bool
	for _, p := range parts {
		if strings.EqualFold(p.Brand, "BOSCH") {
			found = true
			if p.Source != ebaySourceName {
				t.Errorf("Source = %q, want %q", p.Source, ebaySourceName)
			}
			if p.SourceURL == "" {
				t.Errorf("SourceURL empty")
			}
			if p.PriceCents == 0 {
				t.Errorf("PriceCents = 0 (expected parsed price)")
			}
			if p.Currency == "" {
				t.Errorf("Currency empty")
			}
			if p.Condition != "new" {
				t.Errorf("Condition = %q, want %q", p.Condition, "new")
			}
		}
	}
	if !found {
		t.Errorf("expected a Bosch result, none found. parts: %+v", parts)
	}
}

// TestEbayFinder_HttpErrorReturnsErr ensures 5xx surfaces as an error
// (dispatcher swallows it, but the finder itself should not lie).
func TestEbayFinder_HttpErrorReturnsErr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := &http.Client{
		Timeout:   2 * time.Second,
		Transport: &redirectingTransport{target: srv.URL},
	}
	t.Setenv("EBAY_APP_ID", "test-app-id")
	t.Setenv("ONLINE_EBAY_ENABLED", "true")
	f := NewEbayFinder(client)

	_, err := f.Search(context.Background(), "263202G000")
	if err == nil {
		t.Fatalf("expected error on 500 response")
	}
}

// TestEbayFinder_MalformedJSONReturnsNil verifies parse-failure yields
// no results without a panic.
func TestEbayFinder_MalformedJSONReturnsNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`this is not json`))
	}))
	defer srv.Close()

	client := &http.Client{
		Timeout:   2 * time.Second,
		Transport: &redirectingTransport{target: srv.URL},
	}
	t.Setenv("EBAY_APP_ID", "test-app-id")
	t.Setenv("ONLINE_EBAY_ENABLED", "true")
	f := NewEbayFinder(client)

	_, err := f.Search(context.Background(), "263202G000")
	if err == nil {
		t.Errorf("expected parse error on garbage response")
	}
}

// TestExtractBrandFromTitle covers the brand-inference regex against the
// canonical list. Results are post-NormalizeBrand so multi-word variants
// collapse (e.g. "MAHLE ORIGINAL" → "MAHLE" per the M2.S2 canonical map).
func TestExtractBrandFromTitle(t *testing.T) {
	cases := []struct {
		title string
		want  string
	}{
		{"BOSCH F026407008 Oil Filter for Hyundai Sonata 2.4L", "BOSCH"},
		{"Genuine MANN-FILTER W 811/80 for Hyundai Elantra", "MANN-FILTER"},
		{"MAHLE Original OX 254D3 cartridge oil filter", "MAHLE"}, // NormalizeBrand collapses MAHLE ORIGINAL → MAHLE
		{"Some random no-brand seller listing 12345", ""},
		{"HELLA cabin air filter Kia Sportage", "HELLA"},
	}
	for _, tc := range cases {
		got := extractBrandFromTitle(tc.title)
		if !strings.EqualFold(got, tc.want) {
			t.Errorf("extractBrandFromTitle(%q) = %q, want %q", tc.title, got, tc.want)
		}
	}
}

// TestParsePriceFromItem covers the nested-array eBay price envelope.
func TestParsePriceFromItem(t *testing.T) {
	raw := `{
		"sellingStatus":[{
			"currentPrice":[{"__value__":"29.99","@currencyId":"USD"}]
		}]
	}`
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	cents, curr := parsePriceFromItem(m)
	if cents != 2999 {
		t.Errorf("PriceCents = %d, want 2999", cents)
	}
	if curr != "USD" {
		t.Errorf("Currency = %q, want USD", curr)
	}
}

// TestCanonicaliseCondition covers the enumeration mapping.
func TestCanonicaliseCondition(t *testing.T) {
	cases := map[string]string{
		"":                          "unknown",
		"Not Specified":             "unknown",
		"New":                       "new",
		"Brand New":                 "new",
		"Refurbished":               "reman",
		"Remanufactured":            "reman",
		"Rebuilt":                   "reman",
		"Used":                      "used",
		"Pre-owned":                 "used",
		"For parts or not working":  "used",
		"Foobar":                    "unknown",
	}
	for input, want := range cases {
		got := canonicaliseCondition(input)
		if got != want {
			t.Errorf("canonicaliseCondition(%q) = %q, want %q", input, got, want)
		}
	}
}

// redirectingTransport rewrites every outbound request's URL to the test
// server's base URL, preserving path and query.
type redirectingTransport struct {
	target string
}

func (rt *redirectingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	// Rewrite scheme + host to the test server; keep path + query.
	req.URL.Scheme = "http"
	target := strings.TrimPrefix(rt.target, "http://")
	req.URL.Host = target
	req.Host = target
	return http.DefaultTransport.RoundTrip(req)
}

// ebayFixtureBoschOilFilter returns a minimal but valid Finding-API
// envelope containing one Bosch oil-filter item — enough to exercise
// the whole parse path.
func ebayFixtureBoschOilFilter() string {
	return `{
	  "findItemsAdvancedResponse":[{
	    "ack":["Success"],
	    "version":["1.13.0"],
	    "searchResult":[{
	      "@count":"1",
	      "item":[{
	        "itemId":["334455667788"],
	        "title":["BOSCH F026407008 Oil Filter for Hyundai Sonata 2.4L 2013-2019"],
	        "globalId":["EBAY-US"],
	        "primaryCategory":[{"categoryId":["33443"], "categoryName":["Oil Filters"]}],
	        "galleryURL":["https://i.ebayimg.com/thumbs/images/g/xyz/s-l225.jpg"],
	        "viewItemURL":["https://www.ebay.com/itm/334455667788"],
	        "productId":[{"@type":"ReferenceID", "__value__":"140012345"}],
	        "location":["California,USA"],
	        "country":["US"],
	        "shippingInfo":[{"shippingType":["Free"]}],
	        "sellingStatus":[{
	          "currentPrice":[{"__value__":"12.49", "@currencyId":"USD"}],
	          "convertedCurrentPrice":[{"__value__":"12.49", "@currencyId":"USD"}],
	          "sellingState":["Active"]
	        }],
	        "listingInfo":[{"bestOfferEnabled":["false"], "buyItNowAvailable":["false"]}],
	        "condition":[{"conditionId":["1000"], "conditionDisplayName":["New"]}],
	        "isMultiVariationListing":["false"]
	      }]
	    }],
	    "paginationOutput":[{"pageNumber":["1"], "entriesPerPage":["25"]}],
	    "itemSearchURL":["https://www.ebay.com/sch/i.html"]
	  }]
	}`
}
