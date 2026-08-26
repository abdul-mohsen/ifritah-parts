package service

// M8 online-source common types + interface.
//
// Every adapter (eBay Motors, HyundaiPartsDeal, KiaPartsNow, 7zap,
// oilfilter-crossreference, etc.) implements OnlineSource. The federated
// dispatcher (online_search.go) fans out to all enabled sources, dedupes
// results, and caches them.

import (
	"context"
	"time"

	"parts-engine/internal/model"
)

// Trust-tier constants for OnlineSource.TrustScore(). Higher = we trust
// the data more when merging + ranking results across sources.
//
// See docs/data-sources/online-sources-catalog.md for the full rationale
// per-source. When adding a new adapter, pick from these constants
// rather than inventing a new value so ordering stays consistent.
const (
	TrustBrandDirect         = 1.00 // BOSCH / MANN / MAHLE / DENSO / NGK / HELLA first-party
	TrustOfficialAPI         = 0.90 // eBay / AliExpress / Amazon / Walmart / Rakuten
	TrustDealerG5            = 0.85 // HyundaiPartsDeal / KiaPartsNow / Suncoast dealers
	TrustAftermarketRetailer = 0.75 // PartsGeek / CARiD / AutoZone / NAPA / 1AAuto
	TrustRegional            = 0.70 // Emex / Autopedia
	TrustMarketplaceNoisy    = 0.65 // eBay title-inferred / AliExpress seller titles
	TrustCategoryCrossRef    = 0.60 // oilfilter-crossreference.com et al.
)

// OnlineSource is the contract every online-search adapter satisfies.
//
// Enabled() reads an env-var toggle so any single source can be killed
// without a redeploy. RateLimit() returns the minimum interval the
// dispatcher must wait between calls to this source (enforced by a
// shared token-bucket per source name).
//
// TrustScore returns a value in [0, 1] indicating how confidently the
// dispatcher should surface this source's results. See Trust* constants
// above. Sources may return a lower value for marketplace-with-seller-
// data results than they'd return for brand-direct data even if both
// paths run through the same adapter (rare — most adapters have one
// consistent trust tier).
//
// Search() returns aftermarket parts for the given normalised OEM.
// Implementations MUST:
//   - Return (nil, nil) — not an error — when the source is disabled
//     or has no results, so a single sad source does not fail the fan-out
//   - Respect ctx.Done() — bail out fast when the dispatcher cancels
//   - Attach Source and SourceURL to every returned model.AftermarketPart
//     so provenance is preserved through the UNION into
//     FindAftermarketForOEM_Online.
type OnlineSource interface {
	Name() string
	Enabled() bool
	RateLimit() time.Duration
	TrustScore() float64
	Search(ctx context.Context, oemNormalized string) ([]model.AftermarketPart, error)
}
