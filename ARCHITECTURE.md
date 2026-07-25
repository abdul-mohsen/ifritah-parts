# Architecture

## Design principles

1. False positives cost more than false negatives.
2. Every user-facing claim should retain source, confidence, and warnings where practical.
3. Weak/imported evidence must not be presented as OEM-confirmed.
4. Missing media/specification data is safer than decorative or inferred content.
5. Vehicle confirmation is required before a VIN-derived user can browse/search a specific catalog variant.

## Runtime topology

```text
React/Vite frontend
        |
        v
Go HTTP API (Gin) :8080
        |
        +-- PostgreSQL :55432 (Docker)
        |     +-- owned catalog, vehicle linkage, OEM index
        |     +-- substitution links
        |     +-- external source registry
        |     +-- Commons media review queue
        |
        +-- NHTSA vPIC / local vPIC data
        +-- NHTSA recalls API
```

The Go server serves `frontend/dist` when `FRONTEND_DIR` is configured.

## Major components

| Component | Location | Responsibility |
| --- | --- | --- |
| Server wiring | `cmd/server` | Runtime configuration, API route registration, dependencies |
| PostgreSQL/sqlc | `db/migrations`, `db/queries`, `internal/store` | Schema plus generated typed query layer |
| Smart search | `internal/service/smart_search.go` | OEM/article/text/vehicle search, ranking, search confidence |
| Parts lookup | `internal/service/parts_lookup.go` | Catalog parts, vehicle linkage, reverse fitment |
| VIN | `internal/handler/vin.go`, `internal/service/vin_cache.go` | Decode, variants, confirmation state, cache |
| Detail | `internal/handler/parts.go` | Evidence-aware part detail, quality flags, warnings |
| Placement | `internal/service/placement_advisor.go` | Exact/external/inferred/catalog-group placement hierarchy |
| Replacements | `internal/service/replacement*`, `supersession.go` | Cautious shared-OEM and source-backed relationships |
| Recalls | `internal/service/recalls.go` | NHTSA recall fetch/mapping with vehicle-level warning |
| Source governance | `internal/service/external_sources.go` | Registry, source-risk assessment, allowed/rejected source policy |
| Commons review | `internal/service/commons_media.go`, `internal/handler/commons_media.go` | Internal review-first media queue |
| Quality gate | `cmd/qa_gate`, `qa/golden_cases.json` | External-reference API checks and scorecard |
| Browser regression | `frontend/tests/e2e` | VIN, search, catalog, detail, and recall workflow tests |

## Search and fitment flow

```text
Query
  |
  +-- OEM-looking query
  |     +-- owned exact article match
  |     +-- cross-reference merge
  |     +-- exact owned OEM result ranks first
  |
  +-- article-number query
  |     +-- direct catalog article lookup
  |     +-- OEM-format fallback
  |
  +-- confirmed vehicle context
  |     +-- catalog linkage-target search
  |     +-- token-aware part-name matching
  |
  +-- text-only query
        +-- token-aware catalog search
```

When a vehicle linkage target is present, direct owned-catalog records are withheld if no catalog fitment link confirms compatibility. The current in-progress story is extending this proof/status distinction to every returned result.

## Detail evidence model

`GET /api/part/:id/detail` returns:

- Core catalog identity.
- OEM references.
- Vehicle fitment context.
- Replacement candidates with source, confidence, and warnings.
- Placement marked `exact`, `catalog_group`, `inferred`, or `unavailable`.
- `quality.provenanceComplete`, `quality.provenanceGaps`, and individual evidence flags.

Technical specifications currently depend on unavailable migrated TecDoc criteria data. The frontend names this absence and requires pre-fit verification rather than showing empty decorative fields.

## Data-source boundaries

| Source | Allowed role | Not allowed |
| --- | --- | --- |
| Owned PostgreSQL catalog | Parts, linkage fitment, OEM/cross-reference evidence | Claims beyond stored evidence |
| NHTSA vPIC | VIN vehicle identity/context | OEM fitment, part dimensions, replacement claims |
| NHTSA recalls | Safety recall context | VIN-specific confirmation or part-number selection |
| Legacy substitution links | Cautioned reported relationships | OEM-confirmed supersession |
| Retail dealer pages | External QA/reference only | Scraping, ingestion, media reuse |
| Wikimedia Commons | Reviewer-approved generic component illustration | OEM part identity, fitment, dimensions, unreviewed display |

## Media review workflow

Migration `000010_create_commons_media_reviews.sql` stores:

- Generic category, media URL, Commons file page, and source revision.
- License and license URL.
- Required attribution.
- Fixed identity scope: `generic_component`.
- Pending/approved/rejected review state, reviewer, review notes, and timestamps.

Internal endpoints:

- `POST /api/internal/media/commons`
- `GET /api/internal/media/commons`
- `POST /api/internal/media/commons/:id/review`

Only `CC0` and `CC BY 4.0` HTTPS Commons/Creative Commons submissions are accepted. The workflow is deliberately not connected to public detail rendering yet.

## Known architectural constraints

- The old MySQL-only engine resolver was removed. PostgreSQL has no migrated motor-code dataset, and `/api/vehicle/:id/engine` returns `501` with the supported fitment constraint: confirmed vehicle linkage.
- Current external-reference quality metrics are based on a limited golden corpus, not all catalog records.
- Docker PostgreSQL uses host port `55432`, not `5432`.
