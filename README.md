# Parts Engine

Evidence-first Hyundai/Kia parts discovery for two users:

- **Experts:** OEM/part-number lookup with owned-catalog results ranked before aftermarket cross-references.
- **Vehicle owners:** VIN decode, explicit vehicle-variant confirmation, then vehicle-scoped part-name or catalog search.

The product deliberately favors withholding uncertain data over confidently suggesting a wrong part. It does not use fake vehicle/part imagery, scrape dealer catalogs, or call weak replacement links OEM-confirmed.

## Current status

| Area | Status |
| --- | --- |
| Runtime database | PostgreSQL via Docker on `127.0.0.1:55432` |
| Backend | Go, PostgreSQL, sqlc-generated query layer |
| Frontend | React, TypeScript, Vite |
| Local application | `http://127.0.0.1:8080` |
| OEM ranking | Exact owned-catalog OEM records rank before cross-references |
| VIN safety | Ambiguous variants require user confirmation before catalog/part search |
| Recall data | Live NHTSA make/model/year recalls with source and scope warning |
| Real part media | No public OEM media ingestion; internal Commons review workflow only |
| QA quality score | `88.9%` over the current limited external-reference golden set |

## What works

- PostgreSQL-backed vehicle catalog, parts, OEM indexes, substitutions, and source registry.
- VIN decoding from NHTSA vPIC/local data with cached confirmation variants.
- VIN context carried into normal-language search with linkage target, capacity, and fuel type.
- Token-aware natural-language search. `cabin air filter` returns relevant cabin filters and excludes known heater-core/blower false positives for the audited Tucson context.
- OEM query `26300-35505` puts the exact Hyundai/Kia owned-catalog part first.
- Part detail includes source, confidence, fitment list, OEM references, cautious placement, alternatives, replacement evidence, and warnings.
- Legacy substitution links are shown as **source-backed replacement links**, never as OEM-confirmed supersession.
- NHTSA recall notices include source links and explicitly state that model/year matching is not VIN-specific recall confirmation.
- Internal Commons media review queue accepts only reviewed CC0/CC BY 4.0 generic illustrations; no review item is public until separately wired and approved.

## Current quality metrics

Generated report: [`qa/current_impl_quality.json`](qa/current_impl_quality.json)

| Metric | Current |
| --- | ---: |
| System quality score | 88.9% |
| Expected-hit recall, labeled cases | 100% |
| True-negative pass rate, labeled exclusions | 100% |
| Duplicate result rate, returned article numbers | 0% |
| Mean reciprocal rank, exact-ranked cases | 1.0 |
| Recall evidence pass rate | 100% |
| Provenance disclosure accuracy | 100% |
| Provenance completeness | 0% |

`provenanceCompleteness` is intentionally `0`: the audited cabin-filter detail lacks sourced technical specifications. The app exposes this rather than implying dimensions, connectors, torque, or installation attributes it cannot prove.

These are **not corpus-wide accuracy claims**. The golden set currently has a small number of external-reference cases; see [`BUGS.md`](BUGS.md) for the measurement limitations.

## Quick start

```powershell
docker compose up -d postgres

$env:PGHOST = '127.0.0.1'
$env:PGPORT = '55432'
$env:PGUSER = 'parts'
$env:PGPASSWORD = 'parts_engine_pw'
$env:PGDATABASE = 'parts_engine'
$env:PGSSLMODE = 'disable'
$env:DATA_DIR = 'C:\ssda\chatGPT\parts-engine\data'
$env:FRONTEND_DIR = 'C:\ssda\chatGPT\parts-engine\frontend\dist'

go build -o server.exe .\cmd\server
.\server.exe
```

Build the frontend before serving it:

```powershell
Set-Location frontend
npm run build
```

## Validation commands

```powershell
go test ./...

Set-Location frontend
npm run lint
npm run build
npx playwright test

Set-Location ..
$env:QA_REPORT_PATH = 'qa\current_impl_quality.json'
go run .\cmd\qa_gate
```

Latest validated baseline:

- Go: 16 test packages passed across 54 packages.
- Frontend lint and production build passed.
- Playwright: 7 passed.
- Live QA gate: 18 checks passed.

## Delivery status

### In progress

- **Fitment evidence and source coverage:** make every search result distinguish direct catalog-confirmed fitment from contextual or inferred matching, without weakening false-positive protections.

### Pending

- **Part-detail imagery:** render real media only after a reviewer approves a legally reusable, generic illustration. No placeholders, dealer images, scraped OEM diagrams, or unreviewed Commons files may be used.

### Completed milestones

- Initial catalog/detail API and browser safety net.
- PostgreSQL + sqlc migration, Docker runtime, and legacy-cache importer.
- Evidence-aware placement, replacements, substitutions, worker-review isolation, and CI-style QA gate.
- Evidence-first UI redesign; removal of fake vehicle/catalog art.
- Search precision repair, public external QA coverage, VIN-context part search, NHTSA recalls, and cached recall retention.
- OEM-first direct catalog ranking.
- Honest part-detail provenance and removal of unsupported MySQL-only engine filtering.
- Missing-technical-specification guidance.
- Commons review-first media queue.
- Versioned quality scorecard and machine-readable current implementation report.

## Source policy

- NHTSA vPIC: vehicle identity context.
- NHTSA recalls: safety context only; never a part-number selector.
- Owned PostgreSQL catalog / licensed TecDoc-derived data: catalog fitment and cross-reference evidence.
- Hyundai/Kia dealer/retail pages: external QA/reference only, never scraped or ingested.
- Wikimedia Commons: manual review only, generic illustrations only, CC0/CC BY 4.0 only, mandatory attribution; never proof of OEM identity, fitment, or dimensions.

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for design and data boundaries, and [`BUGS.md`](BUGS.md) for known gaps, risks, and the next work.
