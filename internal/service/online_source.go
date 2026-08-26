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

// OnlineSource is the contract every online-search adapter satisfies.
//
// Enabled() reads an env-var toggle so any single source can be killed
// without a redeploy. RateLimit() returns the minimum interval the
// dispatcher must wait between calls to this source (enforced by a
// shared token-bucket per source name).
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
	Search(ctx context.Context, oemNormalized string) ([]model.AftermarketPart, error)
}
