# M5 & M6 Sprint Backlogs

## M5 — Search intelligence (semantic, VIN, related)

**Milestone exit gate:**
- VIN → parts F1 ≥ 0.80
- Description-similarity recall +10 pts on `real_hk_unseeded` slice
- Related-parts suggestion API live

---

### Sprint M5.S1 — Description embeddings

**Task M5.S1.T1 — Embed all TecDoc articles**

**Goal.** Compute a 384-dim embedding for every article's `genericArticleDescription` and store in `pgvector`.

**Files:**
- New `db/migrations/000030_article_embeddings.sql` — `pgvector` extension + column
- New `scripts/embed_articles/main.go` — one-shot batch job
- New `docs/data-sources/embeddings.md`

**Approach:**
1. Enable pgvector extension.
2. Add column `articles_embedding vector(384)` on a new table `article_embeddings` (keep separate to not bloat the shared `articles` table).
3. Embedding model: `sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2` running as a sidecar Python process (via unix socket) — Go binary streams (id, description) → gets vector back.
4. Batch of 1000 at a time. ~27M articles at ~50 embeddings/s = ~150 hours. Run once, incrementally refresh on new imports.
5. Index: `CREATE INDEX ON article_embeddings USING ivfflat (embedding vector_cosine_ops) WITH (lists = 1000);`

**Acceptance criteria:**
- [ ] Migration applies cleanly.
- [ ] Batch job completes for a 10k-sample and produces valid vectors.
- [ ] Sample query `SELECT ... FROM article_embeddings ORDER BY embedding <-> ? LIMIT 10` returns results in < 100 ms.

**Effort:** L

---

**Task M5.S1.T2 — Semantic search endpoint**

**Goal.** New `/api/search/semantic?q=oil filter for Sonata 2020&topK=20` returns top-K articles by cosine similarity.

**Files:**
- New `internal/handler/search_semantic.go`
- New `internal/service/embedding_client.go` (unix-socket client to the embedding sidecar)

**Acceptance criteria:**
- [ ] For 20 natural-language queries (e.g. "brake pads for Elantra 2015", "cabin air filter Tucson", "oxygen sensor Kia Sorento"), top-3 recall against curated ground truth ≥ 0.80.
- [ ] p95 latency ≤ 500 ms.

**Effort:** M

---

### Sprint M5.S2 — VIN → parts pipeline

**Task M5.S2.T1 — VIN decoder**

**Goal.** Given a 17-char VIN, return `(make, model, year, engine, trim)` via offline NHTSA table + HK WMI/VDS rules.

**Files:**
- `internal/service/vin_decoder.go` — expand existing partial implementation
- `internal/service/vin_decoder_test.go`

**Approach:**
1. Offline lookup: `KMH*` → Hyundai, `KNA*` → Kia (WMI).
2. Position 4-8 = VDS (Vehicle Descriptor Section) — model + body + engine code.
3. Position 10 = year code (per SAE J272).
4. Combine with NHTSA VIN decoder API responses stored locally (cached in Postgres).

**Acceptance criteria:**
- [ ] Table-driven test with 30 known HK VINs — all decode correctly.
- [ ] `TestVinDecoder_UnknownWMI` returns nil without panic.

**Effort:** M

---

**Task M5.S2.T2 — Vehicle → parts endpoint**

**Goal.** `/api/vehicle/{vin}/parts?category=filters` returns all HK OEMs cataloged against that vehicle's `linkageTargetId`, grouped by category.

**Files:**
- New `internal/handler/vehicle_parts.go`
- Reuse existing `TecDocVehicle` service

**Approach:**
1. Decode VIN → `linkageTargetId` via existing `ResolveVehicle`.
2. Query `articlelinkages` for all article IDs tied to that linkage target.
3. Group by category (via `DecodeOEMPrefix` on each article's OEM number).
4. Return `map[category][]OEMEntry`.

**Acceptance criteria:**
- [ ] For a known VIN, returns ≥ 30 OEMs across ≥ 10 categories.
- [ ] Category filter narrows the result correctly.

**Effort:** M

---

### Sprint M5.S3 — Related parts recommendations

**Task M5.S3.T1 — Co-occurrence table**

**Goal.** Build a table `related_parts` where each row is `(source_category, related_category, correlation_score, evidence_source)`.

**Files:**
- New `db/migrations/000040_related_parts.sql`
- New `scripts/derive_related_parts/main.go`

**Approach:**
1. Two evidence sources:
   - **Service intervals:** hard-coded Hyundai/Kia service schedules ("at 60,000 km replace: oil filter + air filter + cabin filter + spark plugs + coolant").
   - **User carts:** aggregate observed cart data (once feedback system in M6.S2 exists).
2. Score = frequency of co-occurrence normalised by base frequency of each category.

**Acceptance criteria:**
- [ ] Table populated with ≥ 200 relations from the service-interval evidence.
- [ ] Unit test with `("Oil Filter", …)` returns `("Air Filter", 0.85, "service_60k")` among top results.

**Effort:** M

---

**Task M5.S3.T2 — Related-parts endpoint**

**Goal.** `/api/parts/related?oem=26350-2J001` returns the top-N related-category OEMs for the same vehicle context.

**Files:**
- New `internal/handler/related_parts.go`

**Acceptance criteria:**
- [ ] For 10 known service-bundle categories, related-parts recall ≥ 0.70.

**Effort:** M

---

## M5 exit criteria

- [ ] Embedding table populated + semantic-search endpoint p95 ≤ 500 ms.
- [ ] VIN decoder covers ≥ 30 HK VIN examples.
- [ ] Related-parts endpoint returns useful suggestions for wear-part queries.

---

---

## M6 — Production-grade

**Milestone exit gate:**
- CI blocks any PR that regresses `F1_correct` by ≥ 0.02 on a canary corpus.
- p95 end-to-end latency ≤ 3 s.
- Feedback loop collects thumbs-up/down + aggregates weekly.

---

### Sprint M6.S1 — Continuous audit CI

**Task M6.S1.T1 — Nightly canary audit workflow**

**Goal.** GitHub Actions cron runs `scripts/audit/audit-quality.ps1` against a smaller (200-OEM) canary corpus nightly. Publishes results to `docs/reports/nightly/{YYYY-MM-DD}/`.

**Files:**
- New `.github/workflows/nightly-audit.yml`
- New `scripts/audit/corpus-200-canary.csv`
- New helper `scripts/audit/publish-nightly.ps1`

**Approach:**
1. Cron: `0 3 * * *` (03:00 UTC).
2. Job:
   - Check out repo
   - Set up PowerShell 7
   - Run `audit-quality.ps1` against qa.ifritah.com with `-InputCorpus scripts/audit/corpus-200-canary.csv`
   - Run `analyze-quality.ps1`
   - Commit outputs to a `nightly-audits/{date}` branch
   - Open a PR titled `[nightly-audit] {date}` — auto-merged if F1_correct doesn't regress
3. Fail loud on regression: PR stays open + Slack notification.

**Acceptance criteria:**
- [ ] Workflow runs on schedule successfully once (manual `workflow_dispatch` trigger acceptable).
- [ ] Produces `docs/reports/nightly/{date}/by-category.csv` etc.
- [ ] Commits back cleanly on green.

**Effort:** M

---

**Task M6.S1.T2 — PR quality gate**

**Goal.** Any PR touching `internal/service/` must include an audit-diff. If `F1_correct` on the canary drops ≥ 0.02 vs the last nightly baseline, block the merge.

**Files:**
- New `.github/workflows/pr-quality-gate.yml`

**Approach:**
1. On PR open / update:
   - Skip if only `docs/`, `scripts/`, `frontend/`, `sql/` changed.
   - Otherwise: run the audit against the PR's deployed preview (or dev environment).
   - Compare to the last nightly baseline (fetched from the `nightly-audits/` branches).
   - Fail if `F1_correct` regresses ≥ 0.02.
2. Post the diff as a PR comment (green ✅ / red ❌ per slice).

**Acceptance criteria:**
- [ ] Synthetic "make a regression" PR is blocked.
- [ ] Passing PR merges cleanly.
- [ ] Comment includes the top-5 categories whose F1 changed.

**Effort:** M

---

### Sprint M6.S2 — Feedback loop + cost SLA

**Task M6.S2.T1 — Frontend feedback widget**

**Goal.** "Was this the right part?" thumbs-up/down after every search result. Aggregate weekly.

**Files:**
- `frontend/src/components/ResultFeedback.tsx`
- New `internal/handler/feedback.go`
- New `db/migrations/000050_feedback.sql`

**Schema:**
```sql
CREATE TABLE search_feedback (
    id            BIGSERIAL PRIMARY KEY,
    query_oem     TEXT NOT NULL,
    result_oem    TEXT NOT NULL,
    result_desc   TEXT,
    result_brand  TEXT,
    verdict       TEXT NOT NULL CHECK (verdict IN ('up','down')),
    reason        TEXT,
    submitted_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    session_id    TEXT
);
CREATE INDEX idx_search_feedback_query_oem ON search_feedback(query_oem);
CREATE INDEX idx_search_feedback_verdict ON search_feedback(verdict);
```

**Approach:**
1. On every result card, render `👍 / 👎` buttons.
2. POST to `/api/search/feedback` with the query OEM + result details + verdict + optional reason.
3. Rate-limited to 5 feedbacks per session.

**Acceptance criteria:**
- [ ] Feedback recorded correctly.
- [ ] Weekly aggregation SQL query returns thumbs-up-rate per category.

**Effort:** M

---

**Task M6.S2.T2 — Cost monitoring**

**Goal.** Per-request cost breakdown (DB queries × unit cost, external API calls × unit cost). Alert when p95 request cost > $0.003.

**Files:**
- New `internal/service/cost_meter.go`
- Wire into `SearchWithProgress`
- Emit to a Grafana / Prometheus endpoint

**Approach:**
1. `CostMeter` is a per-request accumulator that tracks:
   - DB queries × 0.0001 USD per query (approximation)
   - External API calls × their published cost
   - Cache hits × 0 (free)
2. Attached to context, incremented by every `logQueryCtx` call and every external HTTP client.
3. On response, emit as an HTTP header `X-Request-Cost-Usd` + log line.

**Acceptance criteria:**
- [ ] p95 cost measurable via a Prometheus scrape.
- [ ] Alert fires when p95 > $0.003 for 5 min.

**Effort:** L

---

## M6 exit criteria

- [ ] Nightly audit workflow green for 7 consecutive days.
- [ ] PR gate blocks a synthetic regression.
- [ ] Feedback widget collecting data for 2 weeks.
- [ ] Cost dashboard live.
- [ ] p95 latency ≤ 3 s on the seeded corpus.

---

## North-star gate (post-M6)

At the end of M6, run the full 1490-OEM audit and verify:

| Metric | Target |
|---|---:|
| `F1_correct` overall | ≥ 0.98 |
| `F1_correct` seeded | ≥ 0.99 |
| `AvgAM_correct` wear parts | ≥ 8 |
| `F1_rich5` wear parts | ≥ 0.90 |
| `F1_rich10` wear parts | ≥ 0.60 |
| p95 latency (mode=combined, enrichment=full) | ≤ 3.0 s |
| Non-HK guard leaks | ≤ 2 of 100 |
| Body/glass `F1_correct` | ≥ 0.90 |

If any metric misses, spin up a M7 remedial milestone before declaring the north-star reached.
