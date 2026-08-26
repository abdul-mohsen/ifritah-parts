package service

// M8 generic G5 (schema.org / OpenGraph public-reference) adapter base.
//
// Many of the 25+ Kia/Hyundai reference sites in the M8 plan follow the
// same shape:
//
//   1. Have a public search endpoint like `/search?q={OEM}`
//   2. Publish product data as schema.org JSON-LD inside `<script>` tags
//   3. Are polite: robots.txt exists and is honoured
//
// GenericG5Adapter implements OnlineSource by wrapping these steps in
// one reusable type. Each real adapter (HyundaiPartsDeal / KiaPartsNow
// / PartsGeek / CARiD / AutoZone / etc.) is 30-60 lines of glue that
// constructs a GenericG5Adapter with the right hostname + URL builder
// + trust score + env-flag name.
//
// This lets us add ~15 adapters cheaply without ~15 copies of HTTP +
// robots + schema.org boilerplate.

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"parts-engine/internal/model"
)

// G5AdapterConfig configures a single G5 site adapter. Every field is
// mandatory; zero-value means the adapter is misconfigured and Enabled()
// returns false.
type G5AdapterConfig struct {
	// Name is the source tag stamped onto every result (e.g. "online:hyundaipartsdeal").
	Name string

	// EnvFlag is the name of the env var that toggles this specific
	// source (e.g. "ONLINE_HYUNDAIPARTSDEAL_ENABLED"). Falsy → disabled.
	EnvFlag string

	// TrustTier — pick from Trust* constants in online_source.go.
	TrustTier float64

	// RateInterval is the minimum wait between consecutive outbound
	// requests to this source's hostname.
	RateInterval time.Duration

	// BuildSearchURL returns the full public URL for looking up an OEM.
	// The adapter respects the URL's hostname for robots.txt lookup;
	// use a canonical scheme (https://).
	BuildSearchURL func(oemNormalized string) string

	// HTTPTimeout caps each outbound request. Default 8 s when zero.
	HTTPTimeout time.Duration
}

// GenericG5Adapter is a reusable OnlineSource implementation over any
// site publishing schema.org JSON-LD product data.
type GenericG5Adapter struct {
	cfg    G5AdapterConfig
	client *http.Client
	robots *RobotsGuard
}

// NewGenericG5Adapter constructs a G5 adapter with the given config and
// shared HTTP client + robots guard. Pass nil for either to use defaults
// (http.Client with cfg.HTTPTimeout or 8s, and a fresh RobotsGuard).
func NewGenericG5Adapter(cfg G5AdapterConfig, client *http.Client, robots *RobotsGuard) *GenericG5Adapter {
	timeout := cfg.HTTPTimeout
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	if robots == nil {
		robots = NewRobotsGuard(nil)
	}
	return &GenericG5Adapter{cfg: cfg, client: client, robots: robots}
}

// Name identifies this source.
func (a *GenericG5Adapter) Name() string { return a.cfg.Name }

// Enabled reads a.cfg.EnvFlag; falsy value ("false", "0", "no") disables.
// Also disabled if BuildSearchURL is nil (misconfiguration).
func (a *GenericG5Adapter) Enabled() bool {
	if a == nil || a.cfg.BuildSearchURL == nil || a.cfg.Name == "" {
		return false
	}
	if a.cfg.EnvFlag == "" {
		return true // no flag → always on
	}
	v := strings.ToLower(strings.TrimSpace(os.Getenv(a.cfg.EnvFlag)))
	if v == "false" || v == "0" || v == "no" {
		return false
	}
	return true
}

// RateLimit returns the configured min interval.
func (a *GenericG5Adapter) RateLimit() time.Duration { return a.cfg.RateInterval }

// TrustScore returns the configured trust tier.
func (a *GenericG5Adapter) TrustScore() float64 { return a.cfg.TrustTier }

// Search runs the G5 lookup: robots.txt guard → HTTP GET → schema.org
// JSON-LD extract → return AftermarketPart list.
func (a *GenericG5Adapter) Search(ctx context.Context, oemNormalized string) ([]model.AftermarketPart, error) {
	if !a.Enabled() || oemNormalized == "" {
		return nil, nil
	}

	url := a.cfg.BuildSearchURL(oemNormalized)
	if url == "" {
		return nil, nil
	}

	// robots.txt compliance BEFORE any HTTP call to the target host.
	allowed, err := a.robots.Allowed(ctx, robotsClientAgent, url)
	if err != nil {
		return nil, fmt.Errorf("%s: robots check: %w", a.cfg.Name, err)
	}
	if !allowed {
		// Silently skip — this is expected behaviour, not an error.
		return nil, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: build request: %w", a.cfg.Name, err)
	}
	req.Header.Set("User-Agent", robotsClientAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: http: %w", a.cfg.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: status %d", a.cfg.Name, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20)) // 2 MB HTML cap
	if err != nil {
		return nil, fmt.Errorf("%s: read: %w", a.cfg.Name, err)
	}

	return ExtractSchemaOrgProducts(body, a.cfg.Name, url)
}

// ─── Concrete adapter constructors ────────────────────────────────────
//
// Each of the following is 5-10 lines wrapping GenericG5Adapter with
// site-specific URL patterns + trust + rate configuration. Adding a new
// site is a one-file follow-up: define constructor, verify the site's
// search URL pattern, register in cmd/api/main.go.

// NewHyundaiPartsDealAdapter — M8.T7. Authoritative Hyundai OEM
// reference. Uses schema.org JSON-LD.
func NewHyundaiPartsDealAdapter(client *http.Client, robots *RobotsGuard) *GenericG5Adapter {
	return NewGenericG5Adapter(G5AdapterConfig{
		Name:         "online:hyundaipartsdeal",
		EnvFlag:      "ONLINE_HYUNDAIPARTSDEAL_ENABLED",
		TrustTier:    TrustDealerG5,
		RateInterval: 2 * time.Second,
		BuildSearchURL: func(oem string) string {
			return "https://www.hyundaipartsdeal.com/search?catchall=" + oem
		},
	}, client, robots)
}

// NewKiaPartsNowAdapter — M8.T8. Authoritative Kia OEM reference.
func NewKiaPartsNowAdapter(client *http.Client, robots *RobotsGuard) *GenericG5Adapter {
	return NewGenericG5Adapter(G5AdapterConfig{
		Name:         "online:kiapartsnow",
		EnvFlag:      "ONLINE_KIAPARTSNOW_ENABLED",
		TrustTier:    TrustDealerG5,
		RateInterval: 2 * time.Second,
		BuildSearchURL: func(oem string) string {
			return "https://www.kiapartsnow.com/search?catchall=" + oem
		},
	}, client, robots)
}

// NewPartsGeekAdapter — M8.T18. Multi-brand aftermarket retailer.
func NewPartsGeekAdapter(client *http.Client, robots *RobotsGuard) *GenericG5Adapter {
	return NewGenericG5Adapter(G5AdapterConfig{
		Name:         "online:partsgeek",
		EnvFlag:      "ONLINE_PARTSGEEK_ENABLED",
		TrustTier:    TrustAftermarketRetailer,
		RateInterval: 2 * time.Second,
		BuildSearchURL: func(oem string) string {
			return "https://www.partsgeek.com/search.html?q=" + oem
		},
	}, client, robots)
}

// NewCARiDAdapter — M8.T19. Specialty performance + body parts.
func NewCARiDAdapter(client *http.Client, robots *RobotsGuard) *GenericG5Adapter {
	return NewGenericG5Adapter(G5AdapterConfig{
		Name:         "online:carid",
		EnvFlag:      "ONLINE_CARID_ENABLED",
		TrustTier:    TrustAftermarketRetailer,
		RateInterval: 2 * time.Second,
		BuildSearchURL: func(oem string) string {
			return "https://www.carid.com/search.html?q=" + oem
		},
	}, client, robots)
}

// NewAutoZoneAdapter — M8.T20. US retailer with Duralast + national brands.
func NewAutoZoneAdapter(client *http.Client, robots *RobotsGuard) *GenericG5Adapter {
	return NewGenericG5Adapter(G5AdapterConfig{
		Name:         "online:autozone",
		EnvFlag:      "ONLINE_AUTOZONE_ENABLED",
		TrustTier:    TrustAftermarketRetailer,
		RateInterval: 2 * time.Second,
		BuildSearchURL: func(oem string) string {
			return "https://www.autozone.com/searchresult?searchText=" + oem
		},
	}, client, robots)
}

// NewAdvanceAutoPartsAdapter — M8.T21.
func NewAdvanceAutoPartsAdapter(client *http.Client, robots *RobotsGuard) *GenericG5Adapter {
	return NewGenericG5Adapter(G5AdapterConfig{
		Name:         "online:advanceautoparts",
		EnvFlag:      "ONLINE_ADVANCEAUTOPARTS_ENABLED",
		TrustTier:    TrustAftermarketRetailer,
		RateInterval: 2 * time.Second,
		BuildSearchURL: func(oem string) string {
			return "https://shop.advanceautoparts.com/find/" + oem
		},
	}, client, robots)
}

// NewNAPAOnlineAdapter — M8.T22.
func NewNAPAOnlineAdapter(client *http.Client, robots *RobotsGuard) *GenericG5Adapter {
	return NewGenericG5Adapter(G5AdapterConfig{
		Name:         "online:napa",
		EnvFlag:      "ONLINE_NAPA_ENABLED",
		TrustTier:    TrustAftermarketRetailer,
		RateInterval: 2 * time.Second,
		BuildSearchURL: func(oem string) string {
			return "https://www.napaonline.com/en/search?query=" + oem
		},
	}, client, robots)
}

// New1AAutoAdapter — M8.T23. Body/suspension direct-to-consumer.
func New1AAutoAdapter(client *http.Client, robots *RobotsGuard) *GenericG5Adapter {
	return NewGenericG5Adapter(G5AdapterConfig{
		Name:         "online:1aauto",
		EnvFlag:      "ONLINE_1AAUTO_ENABLED",
		TrustTier:    TrustAftermarketRetailer,
		RateInterval: 2 * time.Second,
		BuildSearchURL: func(oem string) string {
			return "https://www.1aauto.com/search?keywords=" + oem
		},
	}, client, robots)
}

// NewBuyAutoPartsAdapter — M8.T24.
func NewBuyAutoPartsAdapter(client *http.Client, robots *RobotsGuard) *GenericG5Adapter {
	return NewGenericG5Adapter(G5AdapterConfig{
		Name:         "online:buyautoparts",
		EnvFlag:      "ONLINE_BUYAUTOPARTS_ENABLED",
		TrustTier:    TrustAftermarketRetailer,
		RateInterval: 2 * time.Second,
		BuildSearchURL: func(oem string) string {
			return "https://www.buyautoparts.com/search.aspx?search=" + oem
		},
	}, client, robots)
}

// New7ZapAdapter — M8.T9. Global OEM including MENA.
func New7ZapAdapter(client *http.Client, robots *RobotsGuard) *GenericG5Adapter {
	return NewGenericG5Adapter(G5AdapterConfig{
		Name:         "online:7zap",
		EnvFlag:      "ONLINE_7ZAP_ENABLED",
		TrustTier:    TrustDealerG5,
		RateInterval: 2 * time.Second,
		BuildSearchURL: func(oem string) string {
			return "https://7zap.com/en/search/?q=" + oem
		},
	}, client, robots)
}

// NewEmexAdapter — M8.T32. UAE / GCC marketplace.
func NewEmexAdapter(client *http.Client, robots *RobotsGuard) *GenericG5Adapter {
	return NewGenericG5Adapter(G5AdapterConfig{
		Name:         "online:emex",
		EnvFlag:      "ONLINE_EMEX_ENABLED",
		TrustTier:    TrustRegional,
		RateInterval: 2 * time.Second,
		BuildSearchURL: func(oem string) string {
			return "https://emex.ae/en/search?q=" + oem
		},
	}, client, robots)
}

// NewOilFilterCrossRefAdapter — M8.T36. Filter category cross-ref reference.
func NewOilFilterCrossRefAdapter(client *http.Client, robots *RobotsGuard) *GenericG5Adapter {
	return NewGenericG5Adapter(G5AdapterConfig{
		Name:         "online:oilfilter-crossref",
		EnvFlag:      "ONLINE_OILFILTER_XREF_ENABLED",
		TrustTier:    TrustCategoryCrossRef,
		RateInterval: 5 * time.Second,
		BuildSearchURL: func(oem string) string {
			return "https://oilfilter-crossreference.com/lookup?q=" + oem
		},
	}, client, robots)
}

// NewBoschAftermarketAdapter — M8.T26. First-party BOSCH catalog.
// Uses G5 base though endpoint is brand-owned rather than 3rd-party retail.
func NewBoschAftermarketAdapter(client *http.Client, robots *RobotsGuard) *GenericG5Adapter {
	return NewGenericG5Adapter(G5AdapterConfig{
		Name:         "online:bosch",
		EnvFlag:      "ONLINE_BOSCH_ENABLED",
		TrustTier:    TrustBrandDirect,
		RateInterval: 2 * time.Second,
		BuildSearchURL: func(oem string) string {
			return "https://www.boschaftermarket.com/xrm/net/oemvfl/en/us/searchparts?query=" + oem
		},
	}, client, robots)
}

// NewMannFilterAdapter — M8.T27. First-party MANN-FILTER catalog.
func NewMannFilterAdapter(client *http.Client, robots *RobotsGuard) *GenericG5Adapter {
	return NewGenericG5Adapter(G5AdapterConfig{
		Name:         "online:mann",
		EnvFlag:      "ONLINE_MANN_ENABLED",
		TrustTier:    TrustBrandDirect,
		RateInterval: 2 * time.Second,
		BuildSearchURL: func(oem string) string {
			return "https://www.mann-filter.com/catalog/search?q=" + oem
		},
	}, client, robots)
}

// NewMahleAftermarketAdapter — M8.T28. First-party MAHLE + KNECHT.
func NewMahleAftermarketAdapter(client *http.Client, robots *RobotsGuard) *GenericG5Adapter {
	return NewGenericG5Adapter(G5AdapterConfig{
		Name:         "online:mahle",
		EnvFlag:      "ONLINE_MAHLE_ENABLED",
		TrustTier:    TrustBrandDirect,
		RateInterval: 2 * time.Second,
		BuildSearchURL: func(oem string) string {
			return "https://www.mahle-aftermarket.com/en/products/product-finder?q=" + oem
		},
	}, client, robots)
}

// NewDensoCatalogAdapter — M8.T29. DENSO catalog.
func NewDensoCatalogAdapter(client *http.Client, robots *RobotsGuard) *GenericG5Adapter {
	return NewGenericG5Adapter(G5AdapterConfig{
		Name:         "online:denso",
		EnvFlag:      "ONLINE_DENSO_ENABLED",
		TrustTier:    TrustBrandDirect,
		RateInterval: 2 * time.Second,
		BuildSearchURL: func(oem string) string {
			return "https://www.denso-am.eu/products/product-catalog?q=" + oem
		},
	}, client, robots)
}

// NewHellaCatalogAdapter — M8.T31. HELLA catalog.
func NewHellaCatalogAdapter(client *http.Client, robots *RobotsGuard) *GenericG5Adapter {
	return NewGenericG5Adapter(G5AdapterConfig{
		Name:         "online:hella",
		EnvFlag:      "ONLINE_HELLA_ENABLED",
		TrustTier:    TrustBrandDirect,
		RateInterval: 2 * time.Second,
		BuildSearchURL: func(oem string) string {
			return "https://www.hella.com/parts-catalogue/en/?q=" + oem
		},
	}, client, robots)
}

// NewAutodocAdapter — M8.T38. European aftermarket e-commerce.
//
// Autodoc.co.uk publishes schema.org Product markup on its search
// result pages (SEO requirement — Autodoc is one of the largest
// European auto-parts e-commerce sites, indexed by Google Shopping).
// We consume it as a search endpoint the same way Google indexes it:
// robots.txt-checked, rate-limited, on-demand, User-Agent identified.
//
// Coverage: TecDoc-based; carries every major aftermarket brand
// (BOSCH, MANN, MAHLE, DENSO, VALEO, HELLA, BREMBO, TEXTAR, FEBI,
// LEMFOERDER, LuK, INA, SKF, GATES, Continental, and hundreds more)
// with real-time European inventory + prices in GBP / EUR / SEK / etc.
//
// TrustTier is AftermarketRetailer — data is retailer-published, not
// brand-direct, but authoritative because Autodoc IS a TecDoc consumer
// with contract-verified brand data.
func NewAutodocAdapter(client *http.Client, robots *RobotsGuard) *GenericG5Adapter {
	return NewGenericG5Adapter(G5AdapterConfig{
		Name:         "online:autodoc",
		EnvFlag:      "ONLINE_AUTODOC_ENABLED",
		TrustTier:    TrustAftermarketRetailer,
		RateInterval: 2 * time.Second,
		BuildSearchURL: func(oem string) string {
			return "https://www.autodoc.co.uk/search?keyword=" + oem
		},
	}, client, robots)
}

// NewAutodocDEAdapter — M8.T38b. German-market Autodoc (autodoc.de).
// Different inventory + pricing than the UK site; larger stock. Uses
// the same schema.org markup, different search URL pattern.
func NewAutodocDEAdapter(client *http.Client, robots *RobotsGuard) *GenericG5Adapter {
	return NewGenericG5Adapter(G5AdapterConfig{
		Name:         "online:autodoc-de",
		EnvFlag:      "ONLINE_AUTODOC_DE_ENABLED",
		TrustTier:    TrustAftermarketRetailer,
		RateInterval: 2 * time.Second,
		BuildSearchURL: func(oem string) string {
			return "https://www.autodoc.de/suchen?keyword=" + oem
		},
	}, client, robots)
}

// NewOscaroAdapter — M8.T39. French aftermarket e-commerce.
// Similar shape to Autodoc but France-native. Deep coverage of
// European aftermarket brands for HK vehicles sold in Europe.
func NewOscaroAdapter(client *http.Client, robots *RobotsGuard) *GenericG5Adapter {
	return NewGenericG5Adapter(G5AdapterConfig{
		Name:         "online:oscaro",
		EnvFlag:      "ONLINE_OSCARO_ENABLED",
		TrustTier:    TrustAftermarketRetailer,
		RateInterval: 2 * time.Second,
		BuildSearchURL: func(oem string) string {
			return "https://www.oscaro.com/recherche?searchTerm=" + oem
		},
	}, client, robots)
}

// NewGSFCarPartsAdapter — M8.T40. UK aftermarket retailer.
func NewGSFCarPartsAdapter(client *http.Client, robots *RobotsGuard) *GenericG5Adapter {
	return NewGenericG5Adapter(G5AdapterConfig{
		Name:         "online:gsfcarparts",
		EnvFlag:      "ONLINE_GSFCARPARTS_ENABLED",
		TrustTier:    TrustAftermarketRetailer,
		RateInterval: 2 * time.Second,
		BuildSearchURL: func(oem string) string {
			return "https://www.gsfcarparts.com/search?q=" + oem
		},
	}, client, robots)
}

// NewMicksGarageAdapter — M8.T41. Ireland/UK aftermarket retailer.
// Uses shared TecDoc data so brand coverage is broad.
func NewMicksGarageAdapter(client *http.Client, robots *RobotsGuard) *GenericG5Adapter {
	return NewGenericG5Adapter(G5AdapterConfig{
		Name:         "online:micksgarage",
		EnvFlag:      "ONLINE_MICKSGARAGE_ENABLED",
		TrustTier:    TrustAftermarketRetailer,
		RateInterval: 2 * time.Second,
		BuildSearchURL: func(oem string) string {
			return "https://www.micksgarage.com/search/oem?q=" + oem
		},
	}, client, robots)
}

// NewOnlineCarPartsAdapter — M8.T42. onlinecarparts.co.uk — UK retailer
// with strong TecDoc-sourced aftermarket coverage.
func NewOnlineCarPartsAdapter(client *http.Client, robots *RobotsGuard) *GenericG5Adapter {
	return NewGenericG5Adapter(G5AdapterConfig{
		Name:         "online:onlinecarparts",
		EnvFlag:      "ONLINE_ONLINECARPARTS_ENABLED",
		TrustTier:    TrustAftermarketRetailer,
		RateInterval: 2 * time.Second,
		BuildSearchURL: func(oem string) string {
			return "https://www.onlinecarparts.co.uk/search?keyword=" + oem
		},
	}, client, robots)
}

// AllG5AdaptersDefaultOff returns the full G5 adapter roster with a
// shared http.Client + RobotsGuard. Every adapter is env-flag gated so
// nothing fires unless the operator explicitly enables it. This is the
// recommended shape for wiring in cmd/api/main.go:
//
//	robots := service.NewRobotsGuard(nil)
//	client := &http.Client{Timeout: 8 * time.Second}
//	online := service.NewOnlineSearch(
//	    service.NewAftermarketOnlineCacheRepo(pg),
//	    append(
//	        []service.OnlineSource{service.NewEbayFinder(nil)},
//	        service.AllG5AdaptersDefaultOff(client, robots)...,
//	    )...,
//	)
//	tecdoc = tecdoc.WithOnlineSearch(online)
func AllG5AdaptersDefaultOff(client *http.Client, robots *RobotsGuard) []OnlineSource {
	if robots == nil {
		robots = NewRobotsGuard(client)
	}
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	return []OnlineSource{
		NewHyundaiPartsDealAdapter(client, robots),
		NewKiaPartsNowAdapter(client, robots),
		New7ZapAdapter(client, robots),
		NewPartsGeekAdapter(client, robots),
		NewCARiDAdapter(client, robots),
		NewAutoZoneAdapter(client, robots),
		NewAdvanceAutoPartsAdapter(client, robots),
		NewNAPAOnlineAdapter(client, robots),
		New1AAutoAdapter(client, robots),
		NewBuyAutoPartsAdapter(client, robots),
		NewEmexAdapter(client, robots),
		NewOilFilterCrossRefAdapter(client, robots),
		NewBoschAftermarketAdapter(client, robots),
		NewMannFilterAdapter(client, robots),
		NewMahleAftermarketAdapter(client, robots),
		NewDensoCatalogAdapter(client, robots),
		NewHellaCatalogAdapter(client, robots),
		// European aftermarket retailers — TecDoc-sourced, strong HK coverage
		NewAutodocAdapter(client, robots),
		NewAutodocDEAdapter(client, robots),
		NewOscaroAdapter(client, robots),
		NewGSFCarPartsAdapter(client, robots),
		NewMicksGarageAdapter(client, robots),
		NewOnlineCarPartsAdapter(client, robots),
	}
}
