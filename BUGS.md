# Bugs, Risks, and Quality Gaps

This is the current break-point register. It distinguishes verified defects/limitations from intentionally withheld capabilities.

## Active work

| Item | Status | Impact | Next action |
| --- | --- | --- | --- |
| Per-result fitment evidence | In progress | Some OEM/article result paths expose confidence notes but not a standardized direct/contextual/inferred fitment proof field | Add per-result evidence state/source and regression cases |
| Real part-detail imagery | Pending | No public real part image/diagram is displayed | Wire only reviewer-approved generic Commons illustrations; keep non-part-specific label and attribution |

## Known product/data limitations

| Area | Current behavior | Why it matters |
| --- | --- | --- |
| Technical specifications | Audited part detail has no sourced criteria/dimensions | Users cannot safely infer connectors, dimensions, torque, seals, or installation details |
| Part imagery/diagrams | None in public detail views | No licensed authoritative Hyundai/Kia OEM media feed exists |
| Exact OEM supersession | Only cautious legacy source-backed links | Must not be described as manufacturer-confirmed supersession |
| Engine-code filtering | Explicitly unavailable (`501`) | Motor-code data was not migrated to PostgreSQL; vehicle linkage is the supported constraint |
| Recall scope | Make/model/year only | Not proof that the exact VIN is affected or open for remedy |
| VIN fixtures | Current QA fixture VINs have vPIC check-digit warnings | Replace with checksum-valid public reference VINs before using VIN identity as a release-grade accuracy metric |
| Media review queue | Empty, internal only | No approved file exists and no public rendering is wired |

## QA/measurement limitations

Current report: [`qa/current_impl_quality.json`](qa/current_impl_quality.json)

| Metric limitation | Current state | Required improvement |
| --- | --- | --- |
| Search corpus size | Four search queries, limited positive/negative labels | Add at least 20 externally referenced graded search cases |
| “False-positive/negative rate” | Computed from labeled exclusions/expected hits only | Label a broader corpus and report TP/FP/FN/TN counts |
| Ranking metrics | MRR has one exact-ranked case | Add graded relevance labels and report Precision@K, Recall@K, and nDCG@K |
| Duplicate analysis | Counts repeated returned article numbers | Add canonical OEM-equivalence duplication analysis |
| Cache validation | Checks retained fields across repeat calls | Add cache-hit observability, TTL-expiry, upstream-outage, and concurrency tests |
| Reference validation | Golden cases store public URLs but gate does not fetch/snapshot them | Store retrieval time/hash and add reference availability checks |
| Gate failure accounting | Fatal failures exit before incrementing `checksFailed` | Refactor runner to collect per-case failures into the report |
| Evidence correctness | Mostly validates field presence/caution wording | Add independently verified detail/fitment/placement truth cases |

## Current quality baseline

| Metric | Value | Interpretation |
| --- | ---: | --- |
| System quality score | 88.9% | Limited golden-set score, not a catalog-wide accuracy claim |
| Provenance completeness | 0% | Expected and honest: no sourced technical specifications for audited detail |
| Provenance disclosure accuracy | 100% | The detail payload correctly identifies its missing evidence |
| Labeled expected-hit recall | 100% | All current expected articles were returned |
| Labeled true-negative pass | 100% | Current exclusions were withheld |
| Duplicate article rate | 0% | No repeated article numbers in the current labeled searches |
| Browser regression | 7 passed | Covers mocked VIN confirmation/recall plus live-style catalog/search flows |
| Live API gate | 18 checks passed | Covers current external VIN, search, detail, replacement, substitution, catalog, and recall cases |

## Resolved issues

- Fake vehicle/catalog visuals removed.
- Natural-language cabin-filter search no longer returns audited heater-core/blower false positives.
- Exact owned OEM result `26300-35505` now ranks ahead of aftermarket references.
- Cached VIN responses retain variant-confirmation state and NHTSA recall evidence.
- `provenanceComplete` is no longer hardcoded true.
- Unsupported MySQL-only engine path is removed rather than silently appearing functional.
- Unlicensed/placeholder media cannot enter public detail views; unsafe Commons review submissions are rejected.

## Source safety decisions

- **Rejected for ingestion:** Hyundai/Kia dealer pages, retail diagrams, official marketing/service portal media without a license, and aftermarket web scraping.
- **Allowed as context:** NHTSA vPIC and recalls, with source/scope warnings.
- **Manual review only:** Commons generic illustrations, CC0/CC BY 4.0, mandatory attribution, no fitment/OEM identity claim.

## Restart checklist

1. Read [`README.md`](README.md), [`ARCHITECTURE.md`](ARCHITECTURE.md), and this file.
2. Check `qa/current_impl_quality.json`.
3. Resume `improve-fitment-source-coverage`.
4. Do not begin public imagery until an approved review-queue item exists.
5. Expand the QA corpus before raising quality claims beyond the current scoped score.

## Task ledger

| Status | Tasks |
| --- | --- |
| In progress | `improve-fitment-source-coverage` |
| Pending | `improve-part-detail-imagery` |
| Done: discovery and baseline | `inspect-docs`, `inspect-codebase`, `gather-questions`, `inspect-real-repo-docs`, `inspect-real-repo-code`, `ask-product-questions`, `story-0-baseline` |
| Done: catalog and data foundation | `story-1-part-detail-modal`, `story-1-e2e-safety-net`, `story-2-part-detail-endpoint`, `story-3-external-db`, `story-4-postgres-sqlc`, `docker-postgres-runtime` |
| Done: safety and product flows | `story-5-diagram-experience`, `story-5b-catalog-visual-map`, `story-6-replacement-suggestions`, `story-7-worker-db`, `story-8-quality-gates`, `full-ui-redesign`, `fix-catalog-fake-visual`, `remove-placeholder-art` |
| Done: search, VIN, and evidence | `fix-part-name-search`, `improve-vehicle-confirmation`, `expand-vin-qa-and-nhtsa`, `vin-context-part-search`, `surface-substitution-evidence`, `improve-oem-source-ranking`, `integrate-nhtsa-recalls` |
| Done: quality and governance | `external-qa-audit`, `product-quality-gap-register`, `restore-frontend-quality-gate`, `expand-public-qa-coverage`, `expand-external-qa-recall-coverage`, `correct-provenance-engine-filtering`, `improve-missing-spec-display`, `research-real-part-media`, `commons-media-review-workflow` |
