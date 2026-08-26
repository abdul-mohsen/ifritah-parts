package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestGenericG5Adapter_DisabledByEnvFlag confirms the ONLINE_<SOURCE>_ENABLED
// gate.
func TestGenericG5Adapter_DisabledByEnvFlag(t *testing.T) {
	t.Setenv("ONLINE_TEST_ENABLED", "false")
	a := NewGenericG5Adapter(G5AdapterConfig{
		Name:         "online:test",
		EnvFlag:      "ONLINE_TEST_ENABLED",
		TrustTier:    TrustDealerG5,
		RateInterval: 1 * time.Second,
		BuildSearchURL: func(oem string) string {
			return "https://example.com/search?q=" + oem
		},
	}, nil, nil)

	if a.Enabled() {
		t.Errorf("expected Enabled=false when flag is 'false'")
	}
	parts, err := a.Search(context.Background(), "263202G000")
	if err != nil {
		t.Fatalf("Search err: %v", err)
	}
	if len(parts) != 0 {
		t.Errorf("expected 0 parts when disabled, got %d", len(parts))
	}
}

// TestGenericG5Adapter_MissingURLBuilderDisabled — nil URL-builder
// misconfiguration → Enabled=false, no crash.
func TestGenericG5Adapter_MissingURLBuilderDisabled(t *testing.T) {
	a := NewGenericG5Adapter(G5AdapterConfig{
		Name:      "online:test",
		EnvFlag:   "ONLINE_TEST_ENABLED",
		TrustTier: TrustDealerG5,
	}, nil, nil)
	t.Setenv("ONLINE_TEST_ENABLED", "true")

	if a.Enabled() {
		t.Errorf("expected Enabled=false when BuildSearchURL is nil")
	}
}

// TestGenericG5Adapter_HappyPath — robots-allowed site returns JSON-LD;
// adapter extracts products.
func TestGenericG5Adapter_HappyPath(t *testing.T) {
	fixture := `<html><head>
<script type="application/ld+json">
{
  "@type": "Product",
  "name": "BOSCH Oil Filter for Hyundai Sonata",
  "mpn": "F026407008",
  "brand": "BOSCH",
  "offers": {"@type": "Offer", "price": "12.49", "priceCurrency": "USD"}
}
</script>
</head></html>`

	var searchQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("User-agent: *\nAllow: /\n"))
		case "/search":
			searchQuery = r.URL.Query().Get("q")
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(fixture))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	t.Setenv("ONLINE_TEST_ENABLED", "true")
	a := NewGenericG5Adapter(G5AdapterConfig{
		Name:         "online:test",
		EnvFlag:      "ONLINE_TEST_ENABLED",
		TrustTier:    TrustDealerG5,
		RateInterval: 50 * time.Millisecond,
		BuildSearchURL: func(oem string) string {
			return srv.URL + "/search?q=" + oem
		},
	}, &http.Client{Timeout: 2 * time.Second}, NewRobotsGuard(&http.Client{Timeout: 2 * time.Second}))

	parts, err := a.Search(context.Background(), "263202G000")
	if err != nil {
		t.Fatalf("Search err: %v", err)
	}
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}
	if parts[0].PartNumber != "F026407008" {
		t.Errorf("PartNumber = %q, want F026407008", parts[0].PartNumber)
	}
	if !strings.EqualFold(parts[0].Brand, "BOSCH") {
		t.Errorf("Brand = %q, want BOSCH", parts[0].Brand)
	}
	if parts[0].Source != "online:test" {
		t.Errorf("Source = %q, want online:test", parts[0].Source)
	}
	if searchQuery != "263202G000" {
		t.Errorf("search endpoint received q=%q, want 263202G000", searchQuery)
	}
}

// TestGenericG5Adapter_RobotsDisallowedReturnsEmpty — robots.txt blocks
// our path → adapter returns (nil, nil) without hitting the search
// endpoint.
func TestGenericG5Adapter_RobotsDisallowedReturnsEmpty(t *testing.T) {
	searchHits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /\n"))
		case "/search":
			searchHits++
			w.Write([]byte("should not be reached"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	t.Setenv("ONLINE_TEST_ENABLED", "true")
	a := NewGenericG5Adapter(G5AdapterConfig{
		Name:         "online:test",
		EnvFlag:      "ONLINE_TEST_ENABLED",
		TrustTier:    TrustDealerG5,
		RateInterval: 50 * time.Millisecond,
		BuildSearchURL: func(oem string) string {
			return srv.URL + "/search?q=" + oem
		},
	}, nil, NewRobotsGuard(&http.Client{Timeout: 2 * time.Second}))

	parts, err := a.Search(context.Background(), "263202G000")
	if err != nil {
		t.Fatalf("Search err: %v", err)
	}
	if len(parts) != 0 {
		t.Errorf("expected 0 parts when robots disallows, got %d", len(parts))
	}
	if searchHits != 0 {
		t.Errorf("expected 0 search hits (robots blocked), got %d", searchHits)
	}
}

// TestGenericG5Adapter_5xxReturnsErr — server error surfaces to caller
// (dispatcher will log and skip).
func TestGenericG5Adapter_5xxReturnsErr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			w.Write([]byte("User-agent: *\nAllow: /\n"))
		default:
			http.Error(w, "boom", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	t.Setenv("ONLINE_TEST_ENABLED", "true")
	a := NewGenericG5Adapter(G5AdapterConfig{
		Name:         "online:test",
		EnvFlag:      "ONLINE_TEST_ENABLED",
		TrustTier:    TrustDealerG5,
		RateInterval: 50 * time.Millisecond,
		BuildSearchURL: func(oem string) string {
			return srv.URL + "/search?q=" + oem
		},
	}, nil, NewRobotsGuard(&http.Client{Timeout: 2 * time.Second}))

	_, err := a.Search(context.Background(), "263202G000")
	if err == nil {
		t.Fatalf("expected error on 500 response")
	}
}

// TestAllG5AdaptersDefaultOff_ReturnsAllAdapters — smoke test for the
// registry constructor. Expects 17 adapters (the count declared in
// AllG5AdaptersDefaultOff).
func TestAllG5AdaptersDefaultOff_ReturnsAllAdapters(t *testing.T) {
	adapters := AllG5AdaptersDefaultOff(nil, nil)
	if len(adapters) < 15 {
		t.Errorf("expected ≥15 G5 adapters, got %d", len(adapters))
	}
	// Every adapter has a distinct name.
	seen := map[string]bool{}
	for _, a := range adapters {
		if a.Name() == "" {
			t.Errorf("adapter with empty name: %T", a)
			continue
		}
		if seen[a.Name()] {
			t.Errorf("duplicate adapter name: %s", a.Name())
		}
		seen[a.Name()] = true
	}
	// Every adapter has a positive rate limit + trust tier.
	for _, a := range adapters {
		if a.RateLimit() <= 0 {
			t.Errorf("%s: RateLimit() should be > 0, got %v", a.Name(), a.RateLimit())
		}
		if a.TrustScore() < 0.5 || a.TrustScore() > 1.0 {
			t.Errorf("%s: TrustScore() = %v, want in [0.5, 1.0]", a.Name(), a.TrustScore())
		}
	}
}

// TestAllG5AdaptersDefaultOff_DisabledByDefault — no env flags set → every
// adapter is disabled → no outbound HTTP.
func TestAllG5AdaptersDefaultOff_DisabledByDefault(t *testing.T) {
	// Clear all G5 env flags for this test.
	for _, f := range []string{
		"ONLINE_HYUNDAIPARTSDEAL_ENABLED",
		"ONLINE_KIAPARTSNOW_ENABLED",
		"ONLINE_7ZAP_ENABLED",
		"ONLINE_PARTSGEEK_ENABLED",
		"ONLINE_CARID_ENABLED",
		"ONLINE_AUTOZONE_ENABLED",
		"ONLINE_ADVANCEAUTOPARTS_ENABLED",
		"ONLINE_NAPA_ENABLED",
		"ONLINE_1AAUTO_ENABLED",
		"ONLINE_BUYAUTOPARTS_ENABLED",
		"ONLINE_EMEX_ENABLED",
		"ONLINE_OILFILTER_XREF_ENABLED",
		"ONLINE_BOSCH_ENABLED",
		"ONLINE_MANN_ENABLED",
		"ONLINE_MAHLE_ENABLED",
		"ONLINE_DENSO_ENABLED",
		"ONLINE_HELLA_ENABLED",
	} {
		t.Setenv(f, "false")
	}
	adapters := AllG5AdaptersDefaultOff(nil, nil)
	for _, a := range adapters {
		if a.Enabled() {
			t.Errorf("%s: expected Enabled=false with flag=false, got true", a.Name())
		}
	}
}
