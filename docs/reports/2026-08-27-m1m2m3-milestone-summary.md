# M1 / M2 / M3 Milestone Summary — 2026-08-27

Scope: `abdul-mohsen/ifritah-parts`, `main` at `0f4dce6` (PR #40), post the 15:13–16:45 UTC merge wave.

## M1 Correctness (`F1_correct ≥ 0.95`)

Shipped in prior waves. Aug 27 wave: no M1 tasks landed.

- M1.S1 ranker cross-family penalty — `internal/service/strategy.go` (`strategyCategoryPenalty`, `categoryToSystem` at package init). Shipped via PRs #16–#19 (Aug 21).
- M1.S2 non-HK deny-list widening + confidence floor — `internal/service/hk_scope.go` (deny-list ≥100 entries). Shipped via PR #17 (Aug 21 20:02 UTC).
- M1.S3.T1 category-tokens reverse index — `internal/service/category_tokens.go`. Shipped via PR #20 `c75c85a` (Aug 25 12:48 UTC).
- M1.S3.T2 category-mismatch hard-drop — PR #20. Regressed seeded slice: `F1_correct` 0.71 → 0.59; 54 TPs lost across Cabin Filter / Power Window Motor / Radiator / Air Filter / Thermostat (see PR #21 body, tables).

Open blocker: **PR #21** `fix/m1s3-soft-penalty-instead-of-drop` (`REVIEW_REQUIRED` since Aug 25 19:47 UTC). Reverts hard-drop to `0.5×` confidence penalty on fallback-strategy results with zero token overlap. CI: `build`, `scan`, `detect-changes`, `qa`, `quality-gate` all `SUCCESS`. Target on re-audit: seeded `F1_correct ≥ 0.71`, non-HK leaks `≤ 10/100`.

Exit gate: pending PR #21 merge + post-deploy `pwsh scripts/audit/audit-quality.ps1` re-run.

## M2 Richness (`AvgAM_correct ≥ 5`, `F1_rich5 ≥ 0.60`)

Shipped in prior waves. Aug 27 wave: no M2 tasks landed.

- M2.S1.T1 multi-path aftermarket UNION — `internal/service/tecdoc.go` (`FindAftermarketForOEM` three-path parallel goroutines with 3 s ctx budget, deduped on `NormalizeBrand+PartNumber`). PR #17 `15c79bd` (Aug 21 20:02 UTC).
- M2.S1.T2 supersession-chain aftermarket + M2.S1.T3 brand normalisation — `internal/service/brand_normalize.go`. Same PR chain.
- M2.S2 tiered brand ordering + per-brand cap — `internal/service/alternatives.go` (`maxTotal=20`, `maxPerBrand=3`, tier 1 = Mobis / Bosch / MANN-FILTER / MAHLE / Denso / NGK / Valeo / Hella / Textar / Ferodo / TRW).
- M2.S3 transitive supersession walker — `internal/service/tecdoc_supersession.go` (5-hop cap, cycle guard).

Exit gate: no fresh audit re-run since Aug 25. Deferred to post-#21 deploy + fresh audit.

## M3 Enrichment (`Specs_pct ≥ 80%`, `Vehicles_pct ≥ 60%`, `Supersession_pct ≥ 40%`)

Aug 27 wave landed 3 of 4 M3 tasks.

- **M3.S1.T1** chained article-id promotion — PR #34 (merge `7bfc022`, Aug 27 15:15:30 UTC). Extracts `promoteArticleIds` pipeline. Three-layer fallback (`SearchByOEM → SearchCrossReferences → SearchByOEMIndex`) with canonical `dataSupplierId` pick. Files: `internal/service/enrichment.go` +257/-60, `internal/service/enrichment_test.go` +541 new, `internal/service/tecdoc.go` +68.
- **M3.S1.T2** batched enrichment (`FindSpecificationsBatch`, `FindCompatibleVehiclesBatch`, `FindSupersessionBatch`) — **not shipped**. Requires `articlecriteria.legacyArticleId` index from PR #19 `sql/07` applied first (per `docs/sprints/M3-enrichment.md:105`).
- **M3.S2.T1** `articlesvehicletrees` fallback — PR #40 batch-2 (merge `0f4dce6`, Aug 27 16:45:46 UTC). `FindCompatibleVehicles` retries a 2-way join when the primary 4-way returns zero. Fail-quiet on fallback error.
- **M3.S2.T2** vehicle description parser — PR #33 (merge `ed62ce6`, Aug 27 15:15:05 UTC). Hardens `parseVehicleDescription` against numeric-only parens (`(191)` is HP, not chassis code) and anchors engine-spec regex on decimal displacement. Files: `internal/service/vehicle_description_parser.go` +77/-18, `internal/service/vehicle_description_parser_test.go` +154/-6.

Exit gate: pending M3.S1.T2 (batched enrichment) + fresh `Specs_pct` / `Vehicles_pct` audit.

## Aug 27 wave (context)

Eight PRs merged to `main` between 15:13 and 16:45 UTC: #29 `M0.T4-A`, #30 `M0.T3+M0.T4-B+gap-review`, #31 `M0.T5`, #32 `M6.S1.T1`, #33 `M3.S2.T2`, #34 `M3.S1.T1`, #35 `M0.T2`, #40 `batch-2` (M3.S2.T1 + M6.S1.T2 + M6.S2.T1 + M6.S2.T2). PRs #36–#39 closed at 17:19 UTC without merging; content re-shipped inside #40.

## Next actions

1. Unblock PR #21 review → merge → deploy → certify M1 exit gate via `scripts/audit/audit-quality.ps1` + `analyze-quality.ps1`.
2. Apply `sql/07 articlecriteria.legacyArticleId` index on prod MySQL; then ship M3.S1.T2 batched enrichment.
3. Fresh audit after (1) + (2) to certify M2 richness and M3 `Specs_pct` / `Vehicles_pct` targets on wear parts.
