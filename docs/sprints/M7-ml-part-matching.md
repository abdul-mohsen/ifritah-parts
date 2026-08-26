# M7 — AI/ML part-matching engine

**Milestone owner:** search-quality + ML
**Depends on:** M0 (broken strategies fixed), M4 (aftermarket sources landed), M5.S1 (article embeddings deployed), M6.S2 (feedback events flowing)
**Estimated effort:** 4 sprints (~8-10 weeks at 1 sprint / 2 weeks)

---

## Why M7 is a distinct milestone

M0-M6 close the *structural* gaps in the engine: broken strategies, thin data sources, no learning loop. But even the fully-shipped M0-M6 leaves a large class of queries with **zero direct data**:

- HK OEMs that TecDoc lists but attaches no aftermarket brand to (2026-08-26 diagnostic: top 30 brands on HK cross-refs are all car OEMs — no BOSCH, MANN, MAHLE, DENSO)
- Body / interior / dealer-only parts that RockAuto (M4.S1) can't scrape
- Free-text natural-language queries the current keyword+VIN pipeline can't parse

M7 uses machine learning to close those gaps by **inferring** matches from the abundant training signal already sitting in the database.

## Data foundation (already in place after M0-M6)

| Table | Rows | ML role |
|---|---:|---|
| `articlecrosses` | 30 M | Positive training pairs for cross-brand equivalence |
| `articlecriteria` | 27 M | Spec vectors per article (dim ~30) |
| `articlesvehicletrees` | 340 M | Vehicle-fitment co-occurrence signal |
| `article_embeddings` (M5.S1) | 6.9 M | 384-dim semantic vectors on descriptions |
| `search_feedback` (M6.S2) | growing | Implicit relevance for learned ranker |
| `aftermarket_rockauto` + `aftermarket_community` (M4) | growing | External aftermarket ground-truth |

## What "AI/ML for fitting parts" actually means

Three complementary techniques, each in its own sprint:

### 1. Analogical inference (M7.S1)

If HK OEM `A` has no direct aftermarket data, but part `A` is functionally identical to Toyota OEM `B` (same category, same specs, same vehicle segment), and `B` has 5 aftermarket brands attached — surface those 5 brands for `A` too, tagged as `source=analogy` with a confidence score.

Nearest-neighbor lookup on a fused vector: `(dataSupplierId, assemblyGroupNodeId, spec-vector, description-embedding)`.

### 2. Learned ranker (M7.S2)

Replace hand-tuned strategy priority weights with a LightGBM `LambdaRank` model trained on `search_feedback` (thumbs + click-throughs). Features: strategy source, confidence, brand tier, category-system match, semantic distance, has-specs / has-vehicle flags.

Weekly retrain; regression rollback in the promotion workflow.

### 3. Spec-conditioned equivalence (M7.S3)

For wear parts (filters, brake pads, spark plugs) where a wrong spec means the part physically won't fit — train a siamese network on spec vectors to predict "these two OEMs are functionally equivalent". Gates candidate results before final sort.

### 4. Natural-language queries (M7.S4)

*"oil filter for 2020 Sonata 2.4L"* → LLM-parsed structured query → dispatched to the right pipeline (VIN / semantic / spec-match). Every result carries a one-line "why this?" attribution.

---

## Sprint plan

Each sprint is 2 weeks. Every task ships behind a feature flag so we can A/B test against the current heuristic pipeline.

### M7.S1 — Analogical OEM inference

**Goal:** For an HK OEM with zero direct aftermarket data, surface ≥ 3 inferred aftermarket brands sourced from the nearest analogous OEM in the vector space.

- **M7.S1.T1** — `part_vectors` table + populator
  - **Files:** `db/migrations/000040_part_vectors.sql`, `cmd/build_part_vectors/`
  - **Approach:** for each article, build a 512-dim vector = `(dataSupplierId onehot, assemblyGroupNodeId onehot, top-N criteria as sparse encoding, description embedding from M5.S1)`. Store in pgvector with IVFFlat.
  - **DoD:** 6.9 M rows populated; index built; median build time <2 h; unit test verifies vector shape
  - **Effort:** L

- **M7.S1.T2** — `FindAftermarketByAnalogy` service
  - **Files:** `internal/service/analogy_matcher.go`, wire into `tecdoc.go:FindAftermarketForOEM_MultiPath`
  - **Approach:** when primary + multi-path UNION returns fewer than 3 real aftermarket brands: (1) look up query OEM's vector; (2) k-NN top-20 across all brands filtered to same `assemblyGroupNodeId`; (3) union their `articlecrosses` rows; (4) tag with `source=analogy` + cosine-derived confidence.
  - **DoD:** on 50 seeded HK OEMs known to have zero direct aftermarket rows, ≥ 30 return ≥ 3 inferred brands with cosine ≥ 0.85; no result cosine < 0.70 leaks through
  - **Effort:** L

- **M7.S1.T3** — UI provenance badge
  - **Files:** `frontend/src/components/PartResult.tsx`
  - **DoD:** analogy hits render "Inferred (85% match)"; unit test covers render
  - **Effort:** S

### M7.S2 — Learned ranker (LambdaRank on feedback)

**Goal:** Learned ranker beats hand-tuned priority weights by nDCG@5 ≥ 0.75 on held-out feedback data.

- **M7.S2.T1** — Feature-engineering pipeline
  - **Files:** `cmd/build_training_set/`, `db/migrations/000041_training_features.sql`
  - **Approach:** for every `(query_oem, result_article)` pair in the last 90 days of `search_feedback`, compute features: strategy source, confidence, brand tier, category-system match, semantic distance, has-specs flag, has-vehicle-fitment flag, spec-match count, RockAuto price percentile (if present). Land in `training_features`.
  - **DoD:** ≥ 50 K rows; feature-column completeness > 95%
  - **Effort:** M

- **M7.S2.T2** — Train LightGBM LambdaRank
  - **Files:** `scripts/train_ranker/train.py`
  - **Approach:** Python is fine — offline training is decoupled from the Go runtime. Objective: `LambdaRank` optimising nDCG@5. Hold out 20% for validation. Publish model + feature-importance to `models/ranker-{date}.txt`.
  - **DoD:** nDCG@5 on held-out set ≥ 0.75; feature-importance attached to PR body
  - **Effort:** L

- **M7.S2.T3** — In-Go inference via `leaves`
  - **Files:** `internal/service/ml_ranker.go`, wire into `searchCombined` after dedupe, before sort
  - **Approach:** `leaves` is a pure-Go LightGBM inference library. Load model at startup. A/B toggle `USE_ML_RANKER=true/false`.
  - **DoD:** p95 latency impact <50 ms; toggle-off matches current behaviour byte-for-byte; A/B split lands in feedback with model version tagged
  - **Effort:** M

- **M7.S2.T4** — Weekly retrain + regression rollback
  - **Files:** `.github/workflows/ml-ranker-retrain.yml`, `scripts/train_ranker/promote.py`
  - **Approach:** cron rebuilds feature set + retrains + validates against previous week's held-out set. If nDCG@5 regresses > 0.02, reject new model + alert.
  - **DoD:** cron runs weekly; regression rollback exercised in dry-run PR
  - **Effort:** M

### M7.S3 — Spec-conditioned equivalence (wear parts)

**Goal:** Siamese network AUROC ≥ 0.92 on held-out `articlecrosses` pairs; false-positive rate <5% at operating threshold.

- **M7.S3.T1** — Mine positive/negative pairs from `articlecrosses`
  - **Files:** `cmd/mine_equivalence_pairs/`, `db/migrations/000042_equivalence_training_pairs.sql`
  - **Approach:** every `articlecrosses` row = a `(oem_A, oem_B, label=1)` positive pair. Negatives sampled from `(oem_A, oem_random_same_category, label=0)` at 4:1 ratio.
  - **DoD:** ≥ 500 K pairs generated; label balance verified
  - **Effort:** M

- **M7.S3.T2** — Train siamese network
  - **Files:** `scripts/train_equivalence/train.py`
  - **Approach:** 2-layer MLP over pair of `part_vectors` (from M7.S1.T1); contrastive loss; Adam optimiser.
  - **DoD:** AUROC ≥ 0.92 on held-out; FPR < 5% at threshold
  - **Effort:** L

- **M7.S3.T3** — In-Go equivalence-scorer + filter
  - **Files:** `internal/service/equivalence_scorer.go`
  - **Approach:** load ONNX-exported siamese model; score candidate pairs; filter out results below operating threshold before final sort.
  - **DoD:** wear-part `F1_correct` climbs by ≥ 5 pts on canary; behind `USE_ML_EQUIVALENCE=true` flag
  - **Effort:** M

### M7.S4 — Natural-language queries

**Goal:** Support free-text queries with nDCG@5 ≥ 0.70 against human-curated top-5 on 20 real-world queries.

- **M7.S4.T1** — LLM query parser
  - **Files:** `internal/service/nl_query_parser.go`, `internal/service/llm_client.go`
  - **Approach:** small model (Llama 3 8B via ollama or gpt-4o-mini via API); structured-output JSON constraint; extracts `(part_type, make, model, year, engine, brand_pref, tier_pref)`; cache identical queries 24h.
  - **DoD:** for 100 real-world natural-language queries from parts-seller logs, parser field-recall ≥ 0.90
  - **Effort:** M

- **M7.S4.T2** — Query router
  - **Files:** `internal/service/query_router.go`
  - **Approach:** after parsing, dispatch to (a) VIN pipeline if VIN, (b) semantic + vehicle-fitment JOIN if part_type + vehicle, (c) full-text fallback otherwise.
  - **DoD:** 20/20 correct routing on the test set; nDCG@5 ≥ 0.70
  - **Effort:** M

- **M7.S4.T3** — Result provenance / explainability
  - **Files:** `frontend/src/components/PartResult.tsx`
  - **Approach:** every NL-query result carries a one-line "why this?" attribution (matched via spec / VIN / analogy / etc.).
  - **DoD:** 100% of NL results have attribution; A/B trial +8 pts thumbs-up rate
  - **Effort:** S

---

## Exit gate for M7

Full audit re-run with:

| Metric | Baseline (post-M6) | M7 target |
|---|---:|---:|
| `AvgAM_inferred` on 50-OEM zero-direct-data canary | 0 | **≥ 3** |
| Ranker nDCG@5 vs hand-tuned priority (held-out feedback) | — | **≥ 0.75** |
| Wear-part `F1_correct` on canary (equivalence filter on) | — | **+5 pts vs M6** |
| NL-query parser field-recall on 100 parts-seller logs | — | **≥ 0.90** |
| NL-query nDCG@5 vs human-curated top-5 (20 queries) | — | **≥ 0.70** |
| p95 latency (all M7 features on) | ≤ 3.0 s | **≤ 3.2 s** (200 ms budget for ML) |
| Non-HK guard leak count (regression check) | ≤ 2/100 | **≤ 2/100** |

---

## Risks

| Risk | Mitigation |
|---|---|
| Analogy returns wrong-category parts | Gate by `assemblyGroupNodeId` match (post-M3) + cosine ≥ 0.85 + explicit "inferred" tag |
| Learned ranker overfits to narrow user cohort | 90-day sliding retrain window; nDCG regression rollback |
| LLM parser hallucinates trims / years | JSON-schema constraint; validate against NHTSA-derived VIN patterns |
| ML infra cost exceeds $0.003/request SLA | 24h NL-query cache, 30d embedding cache; fallback to keyword when hot-path >200 ms |
| Stale models served post-refresh | Model version stamped into every result; audits pinned to model file |
| ONNX / `leaves` runtime instability in Go | Feature-flag every M7 feature; toggle-off reverts to M6 heuristic pipeline |

---

## Non-goals for M7

- Training a foundation model — always use pretrained embeddings + downstream fine-tuning
- Real-time gradient updates — every model retrained offline on a cron
- Removing the heuristic pipeline — M7 augments, doesn't replace. The heuristic layer is the fallback when the ML layer is off, uncertain, or regressed.
