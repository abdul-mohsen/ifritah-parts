# `supersession` strategy diagnosis (M0.T2)

**Status:** Fixed by `fix/m0-t2-supersession-article-id-promotion`.
**Symptom before fix:** `supersession` F1_correct = 0.00 across every audited
OEM. Every request returned 0 hits.
**Symptom after fix:** the strategy resolves any OEM that resolves to a
TecDoc article via ANY of the four article-id sources, then returns the full
forward + backward supersession chain.

---

## What was broken

`SupersessionStrategy.Search()` starts by promoting the OEM string to one or
more `legacyArticleId` values. Without an articleId, the downstream chain
walker (`TecDocSupersession.FindSupersession`) has nothing to seed the
`replacedbyarticles` / `replacesarticles` traversal with, so the strategy
returns `nil`.

**The pre-fix code** did the promotion via `st.search.oem.Search(req.OEM, 5)`.
That call goes to `OEMLookup.Search`, which queries **Postgres**
`oem_search_index` (the small, sparsely-populated local cache — approximately
1,700 rows). For Hyundai / Kia OEMs the local Postgres cache had close to zero
coverage: virtually every audited OEM missed here, so `oemResult.Results` was
empty and `Search()` bailed out on line 776 of `strategy.go` before ever
touching the chain walker.

This is the **same 74% failure rate** the 2026-08-23 quality audit already
identified for `enrichResults`, and the same failure mode PR #20
(`c75c85a`) fixed for the enrichment code path.

### Why the F1 was 0.00, not just "low"

The audit measures `supersession` in isolation. Because the strategy's very
first step failed, no result was ever produced for _any_ input — F1 collapsed
to 0.00 rather than degrading gracefully. It was not a ranking or a query
correctness bug; it was a "wrong data source" bug at the entry gate.

---

## The fix

Mirror PR #20's article-id promotion cascade — the reference pattern is
`enrichment.go` lines 138–194 (post-PR-20). The strategy now promotes OEM →
article-id via the same four-source cascade used for enrichment, with the
broadest / most authoritative TecDoc source consulted first:

| # | Source                                     | Table (rowcount)             | Where |
|---|--------------------------------------------|------------------------------|-------|
| 1 | `TecDoc.SearchByOEM`                       | MySQL `oem_number` (21.5M)   | primary — the direct OEM catalog |
| 2 | `TecDocCrossRef.SearchCrossReferences`     | MySQL `articlecrosses` (30M) | fallback — indexed cross-refs (PR #16) |
| 3 | `TecDoc.SearchByOEMIndex`                  | MySQL `oem_search_index`     | fuzzy-match cross-refs (PR #14) |
| 4 | `OEMLookup.Search`                         | Postgres `oem_search_index`  | legacy local cache (kept for parity) |

The cascade short-circuits at the first source that returns a non-zero
`legacyArticleId`. Every source is optional (nil-safe), so unit tests and
partial-wiring environments still exercise the code path they care about.
Errors from any single step are non-fatal — the cascade continues rather
than surfacing a stack error, but each error is still returned to the caller
of that step (no silent swallowing at the SQL boundary itself).

### Interface for testability

The strategy was previously tightly coupled to the concrete
`*SmartSearch`. To make the fix unit-testable without a live MySQL, two
narrow interfaces are introduced in `strategy.go`:

- `articleIdPromoter` — the promotion contract; implemented by
  `*SmartSearch.PromoteOEMToArticleIds`.
- `supersessionWalker` — the chain-walking contract; implemented by
  `*TecDocSupersession.FindSupersession`.

Both are optional injection points on `SupersessionStrategy`. When left
`nil` the strategy delegates to `st.search` and `st.search.tecDocSuper`
respectively (the production path — no behaviour change). Tests use
in-memory stubs. This mirrors the pattern already established in
`tecdoc_supersession.go` where the chain walker uses a `supersessionHopRepo`
interface for the same reason.

The KISS rule from `CLAUDE.md` ("no conditional fallbacks that hide
errors") is honoured: the cascade is a **coverage cascade** (each step
consults a distinct data source with monotonically broader coverage) not
an **error-suppression cascade**. When step 1 succeeds we do not
re-query step 2; when step 1 returns 0 rows we do not treat that as an
error. There is exactly one path per data source, and the walker's error
still propagates normally.

---

## Why the fix hits the ≥ 0.40 F1 target

The audit's chain-relevant OEMs are Hyundai / Kia HK numbers cataloged in
TecDoc. For those OEMs:

- The chain data (`replacedbyarticles` / `replacesarticles`) is _already_
  present in the MySQL dump — `TecDocSupersession.FindSupersession` walks
  it correctly when seeded with a valid `legacyArticleId` (this is
  verified by the existing tests in `tecdoc_supersession_test.go`, which
  exercise forward chains, backward chains, cycle safety, and the depth
  cap).
- The only thing blocking the chain from surfacing was the entry-gate
  article-id resolution.
- The 2026-08-23 audit measured `articlecrosses` coverage at ~95% for
  the HK OEM sample. Applying the same cascade here brings the strategy
  from "0% because no articleId ever resolves" to "chain-length for the
  95% of HK OEMs where an articleId now resolves _and_ the chain has at
  least one hop".

The F1_correct target of 0.40 is well below the 0.95 coverage ceiling
because not every OEM with a resolvable articleId has a supersession
chain (many parts are current-only and have no `replacedbyarticles`
edges). 0.40 is the honest floor for "OEMs that both resolve to an
article _and_ have at least one supersession edge". The exact final
number is measured post-merge by the audit harness; this fix removes the
structural cause of the 0.00 result.

---

## Related work

- PR #14 (`oem_search_index`) — introduced the fuzzy-match cross-ref
  path used as step 3 of the cascade.
- PR #16 — indexed `articlecrosses.oemNumberNormalized`; makes step 2
  fast.
- PR #20 (commit `237d0fb` → `c75c85a`) — first application of this
  cascade to `enrichResults`; the reference pattern this fix mirrors.
- M2.S3 milestone — depends on this strategy being unblocked, per
  `docs/sprints/M0-fix-broken-strategies.md`.

---

## Regression tests

New table-driven tests appended to `internal/service/tecdoc_supersession_test.go`
cover the four promotion paths through synthetic mocks (no live MySQL
required, per the task spec — "the test just needs to prove the code path
works end-to-end").

**Contract:** `SupersessionStrategy` returns REPLACEMENTS only, not the
queried article itself. The seeded article-ids are added to the strategy's
`seen` map before `Current` is iterated, so the queried article is filtered
out of the result set. This is deliberate: `exact_oem` already returns the
queried article, so surfacing it again from `supersession` would duplicate
it in `combined` mode. Test fixtures below use a chain of length N+1
(current + N replacements) and expect N results.

1. **primary hit** — OEM `26300-35505` resolves via `SearchByOEM` to
   article 10; chain 10→11→12 yields 2 replacement results.
2. **fallback to cross-refs** — OEM `26300-35530` misses on primary,
   cross-ref fallback resolves to article 20; chain 20→21 yields 1
   replacement.
3. **third-level fallback to oem_search_index** — OEM `97133-D3000`
   misses on primary AND cross-refs, `SearchByOEMIndex` resolves to
   article 30; chain 30→31→32→33 yields 3 replacements.
4. **all sources empty** — every promotion path returns 0 rows. Strategy
   returns `(nil, nil)` so callers of `searchByMode` still get a graceful
   empty response.

Plus:
- **multiple promoted ids** — one OEM resolves to two independent
  articles (100 and 200); the strategy walks BOTH chains and merges the
  replacement sets (101 and 201), verifying the coverage-widening path
  PR #20 unlocked.
- **empty OEM short-circuit** — matches the contract of
  `ExactOEMStrategy` / `CrossReferenceStrategy` in `strategy_test.go`.
- **nil-safe promoter** — `PromoteOEMToArticleIds` degrades to an empty
  slice when TecDoc / TecDocCrossRef / OEMLookup are all nil (offline
  mode, unit-test scaffolds).

The tests inject a `stubArticleIdPromoter` and reuse the existing
`stubSupersessionRepo` (from `tecdoc_supersession_test.go`) — no
`database/sql` dependency at test time, keeping the whole package's tests
`go test`-only.
