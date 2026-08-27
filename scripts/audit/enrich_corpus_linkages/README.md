# enrich_corpus_linkages

**Milestone:** M0.T4 sub-B (the second half of PR #29 — `fix/m0-t4-catalog-vehicles-case-insensitive`).

## What it does

Reads an audit corpus CSV (e.g. `scripts/audit/corpus-1500-v2.csv`) and augments each row with two new columns:

- `LinkageTargetIds` — up to `-k` linkage target ids for the OEM, discovered by walking `oem_number → articles → articlesvehicletrees` (type `P`), pipe-separated.
- `SeedArticleIds` — up to `-k` `legacyArticleId` candidates ordered by `dataSupplierId DESC` (rough proxy for the most-canonical supplier catalog).

Both columns are populated by querying the **TecDoc MySQL** corpus. The tool is standalone and does not link into the server binary — so it can be exercised in CI or against a local dump without touching the runtime.

## Why the runtime audit needs it

The 2026-08-24 per-strategy audit found `vehicle_fitment` and `spec_match` returning 0 hits on every corpus row because those strategies require a `linkageTargetId` / `seedArticleId` parameter that the corpus did not carry. `M0.T4` splits the fix into:

- **Sub-A (PR #29):** case-insensitive `/api/catalog/vehicles` so any user obtaining an id from the frontend gets a non-empty response.
- **Sub-B (this tool):** back-fill the audit corpus with real ids so per-strategy F1 numbers can be measured.

Both halves are needed for the milestone to close.

## Usage

```bash
# With a live MySQL connection — real enrichment
go run ./scripts/audit/enrich_corpus_linkages \
    -corpus  scripts/audit/corpus-1500-v2.csv \
    -out     scripts/audit/corpus-1500-v2.enriched.csv \
    -mysql   "user:pass@tcp(host:3306)/tecdoc?parseTime=true&charset=utf8mb4" \
    -k       5
```

```bash
# Shape-check mode — validates the CSV layout without needing credentials.
# Every row gets appended with empty LinkageTargetIds + SeedArticleIds
# columns; useful in CI to guard against a corpus-header regression.
go run ./scripts/audit/enrich_corpus_linkages \
    -corpus  scripts/audit/corpus-1500-v2.csv \
    -out     /tmp/shape-check.csv
```

### Flags

| Flag         | Default | Purpose                                                                 |
| ------------ | ------- | ----------------------------------------------------------------------- |
| `-corpus`    | *(req)* | Input CSV. Must contain a column named `OEM`.                           |
| `-out`       | *(req)* | Output CSV path; `-` writes to stdout.                                  |
| `-mysql`     | `""`    | TecDoc MySQL DSN. Empty string = shape-check mode (no queries).         |
| `-k`         | `5`     | Max linkage ids and seed article ids to record per OEM. Range: 1..50.   |
| `-id-sep`    | `\|`    | Separator inside the two new columns. `\|` is CSV-safe (no quoting).    |
| `-skip-header` | `false` | Treat the input as headerless and synthesize an `OEM`-only header.    |

## Exit codes

| Code | Meaning                                                              |
| ---: | -------------------------------------------------------------------- |
| `0`  | Success — enriched rows written; summary emitted on stderr.          |
| `1`  | Input error (missing corpus, malformed header, unreadable file).     |
| `2`  | Database error (connection failed, query failed on all rows).        |

## Summary stats

The tool emits a one-line summary on stderr for every run:

```
enrich_corpus_linkages: total=1500 enriched=940 no_linkage=520 errors=40
```

- `total` — corpus rows read
- `enriched` — rows with at least one linkage id resolved
- `no_linkage` — rows where the OEM exists in `oem_number` but no `articlesvehicletrees` row exists
- `errors` — rows where at least one of the two queries returned a non-`sql.ErrNoRows` error

## Downstream consumption

Once the enriched CSV is committed, `scripts/audit/audit-quality.ps1` needs a follow-up patch to append `&linkageTargetId=<id>` when `Mode == vehicle_fitment` (the corpus row now has the id). That patch is **not** part of this tool — it lives in the audit harness. Tracked as **M0.T6** (corpus enrichment → per-strategy audit wiring).

## Not covered

- Retry / rate-limiting: TecDoc queries are direct MySQL — the driver's connection pool handles concurrency. `perOEMTimeout` (15 s) is per-OEM; a bad plan on one row cannot stall the whole run.
- Cache-invalidation of the enriched CSV: manual re-run. The corpus is expected to change rarely (~monthly).
- The strategy code path itself: that is `internal/service/strategy_assembly.go:VehicleFitmentStrategy` — see M0.T3 for the sibling `vin_assembly` fix.
