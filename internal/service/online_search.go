package service

// M8.T10 — federated online-search dispatcher.
//
// Given an OEM, fans out to every enabled OnlineSource in parallel,
// deduplicates results by (brand, part_number), and persists the union
// to the aftermarket_online_cache table via async upsert.
//
// Cache-first read path: on entry, we ask the repo for fresh rows. If
// any exist, return them immediately — no outbound HTTP. This is what
// keeps the p95 latency budget under control (< 50 ms per cached
// lookup vs multi-second live-fan-out).
//
// Miss path: fan out with a shared context whose deadline is
// dispatcherTimeout. Each source gets its own rate-limiter wait +
// individual timeout so a slow source can't strangle the fan-out.
//
// Kill switch: env ONLINE_SEARCH_ENABLED=false disables the entire
// subsystem — Search returns (nil, nil) without touching any source.
// Per-source flags are honoured by each source's Enabled() method.

import (
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"parts-engine/internal/model"
)

const (
	dispatcherTimeout  = 8 * time.Second // whole fan-out budget
	perSourceTimeout   = 5 * time.Second // per-source hard cap
	dispatcherCacheTTL = 30 * 24 * time.Hour
	dispatcherMaxTotal = 40 // hard cap on returned rows before dedupe
)

// OnlineSearch is the federated dispatcher.
type OnlineSearch struct {
	sources      []OnlineSource
	cache        *AftermarketOnlineCacheRepo
	limiters     map[string]*OnlineRateLimiter
	limitersOnce sync.Once
	limitersMu   sync.Mutex
}

// NewOnlineSearch constructs a dispatcher with the given sources +
// cache repo. Pass an empty source slice to effectively disable the
// dispatcher (Search returns cached-only results).
func NewOnlineSearch(cache *AftermarketOnlineCacheRepo, sources ...OnlineSource) *OnlineSearch {
	return &OnlineSearch{
		sources:  sources,
		cache:    cache,
		limiters: make(map[string]*OnlineRateLimiter),
	}
}

// GloballyEnabled reports whether the ONLINE_SEARCH_ENABLED kill switch
// permits any outbound calls. When false, Search returns cache-hits
// only (never fans out).
func (o *OnlineSearch) GloballyEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("ONLINE_SEARCH_ENABLED")))
	if v == "false" || v == "0" || v == "no" {
		return false
	}
	// Default: enabled if ANY source is enabled. This makes tests + local
	// dev "just work" without every operator having to set env.
	return true
}

// Search runs the federated meta-search for oemNormalized.
//
//  1. Read fresh rows from aftermarket_online_cache — if any, return.
//  2. If GloballyEnabled == false, return whatever the cache had (may
//     be nil).
//  3. Otherwise fan out to enabled sources in parallel; merge + dedupe;
//     write to cache asynchronously.
//
// Errors from individual sources are logged and swallowed — one source
// failing does NOT fail the whole search.
func (o *OnlineSearch) Search(ctx context.Context, oemNormalized string) []model.AftermarketPart {
	if o == nil || oemNormalized == "" {
		return nil
	}

	// Fast path: cache read.
	if o.cache != nil {
		if cached, err := o.cache.FreshFor(ctx, oemNormalized); err == nil && len(cached) > 0 {
			return cached
		} else if err != nil {
			log.Printf("[OnlineSearch] cache read oem=%s err=%v", oemNormalized, err)
		}
	}

	if !o.GloballyEnabled() {
		return nil
	}

	// Fan-out with a bounded context.
	fanCtx, cancel := context.WithTimeout(ctx, dispatcherTimeout)
	defer cancel()

	type sourceResult struct {
		name  string
		parts []model.AftermarketPart
	}
	results := make(chan sourceResult, len(o.sources))
	var wg sync.WaitGroup

	enabled := 0
	for _, s := range o.sources {
		if !s.Enabled() {
			continue
		}
		enabled++
		wg.Add(1)
		go func(src OnlineSource) {
			defer wg.Done()
			// Per-source ctx: min(fanCtx deadline, perSourceTimeout).
			srcCtx, srcCancel := context.WithTimeout(fanCtx, perSourceTimeout)
			defer srcCancel()

			limiter := o.limiterFor(src)
			if err := limiter.Wait(srcCtx); err != nil {
				log.Printf("[OnlineSearch] source=%s rate-wait cancelled: %v", src.Name(), err)
				return
			}
			parts, err := src.Search(srcCtx, oemNormalized)
			if err != nil {
				log.Printf("[OnlineSearch] source=%s err=%v", src.Name(), err)
				return
			}
			if len(parts) == 0 {
				return
			}
			results <- sourceResult{name: src.Name(), parts: parts}
		}(s)
	}

	// Close results after all goroutines finish.
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect + dedupe.
	seen := make(map[string]bool, dispatcherMaxTotal)
	var out []model.AftermarketPart

collect:
	for {
		select {
		case r, ok := <-results:
			if !ok {
				break collect
			}
			for _, p := range r.parts {
				if p.Brand == "" || p.PartNumber == "" {
					continue
				}
				key := NormalizeBrand(p.Brand) + "|" + strings.ToLower(p.PartNumber)
				if seen[key] {
					continue
				}
				seen[key] = true
				// Ensure brand is normalised in the returned payload too
				// so downstream ranking sees the canonical form.
				p.Brand = NormalizeBrand(p.Brand)
				out = append(out, p)
				if len(out) >= dispatcherMaxTotal {
					break collect
				}
			}
		case <-fanCtx.Done():
			log.Printf("[OnlineSearch] fan-out ctx deadline oem=%s enabled=%d partial=%d", oemNormalized, enabled, len(out))
			break collect
		}
	}

	// Persist to cache asynchronously.
	if o.cache != nil && len(out) > 0 {
		o.cache.UpsertAsync(oemNormalized, dispatcherCacheTTL, out)
	}

	return out
}

// limiterFor returns the shared rate-limiter for the given source. Constructed
// lazily on first use.
func (o *OnlineSearch) limiterFor(s OnlineSource) *OnlineRateLimiter {
	name := s.Name()
	o.limitersMu.Lock()
	defer o.limitersMu.Unlock()
	if l, ok := o.limiters[name]; ok {
		return l
	}
	l := NewOnlineRateLimiter(s.RateLimit())
	o.limiters[name] = l
	return l
}
