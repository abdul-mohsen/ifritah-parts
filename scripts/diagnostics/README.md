# M0 Broken-Strategy Diagnostics + TecDoc Baseline Check

The full TecDoc audit completed 2026-08-27. Detailed findings are pinned in the
agent's local scratch (not in the repo). What remains here is:

1. **`tecdoc_diagnostic_full.sql`** — the trimmed baseline regression check (2 sections, ~2 seconds) to re-run whenever a new migration is applied, a TecDoc dump is refreshed, or `FindBySpecMatch` performance regresses in prod. Confirms every P0 index is present and every hot query hits it.

2. The M0 drill-down scripts for the four broken strategies. Only run these when the corresponding strategy shows 0 hits in an audit — they're deeper investigations, not routine.

## Files

| Task | Database | Script |
|---|---|---|
| **TecDoc baseline regression check (routine)** | **qa MySQL / MariaDB (TecDoc)** | **`tecdoc_diagnostic_full.sql`** |
| M0.T1 owned_catalog drill-down | qa Postgres | `owned_catalog_postgres.sql` |
| M0.T2 supersession drill-down | qa MySQL (TecDoc) | `supersession_mysql.sql` |
| M0.T3 vin_assembly | qa Postgres + code trace | `vin_assembly_diagnosis.md` |
| M0.T4 vehicle_fitment drill-down | qa MySQL (TecDoc) + Postgres | `vehicle_fitment_mysql.sql` |
| Live SSE strategy capture | live SSE endpoint | `capture_debug_logs.sh` |

## How to run

### `tecdoc_diagnostic_full.sql` — baseline regression check

Two sections:

- **§A** — P0 index PASS/FAIL (`idx_articlecrosses_oemNumberNormalized`, `idx_oem_number_clean_number`, `idx_articlecriteria_legacyArticleId`, `idx_articlecriteria_criteria_value`, and the `oemNumberNormalized` generated column). All 5 should say `PRESENT`.
- **§F1-§F6** — `EXPLAIN` on every hot production query (`FindBySpecMatch`, `FindSpecifications`, `SearchCrossReferences`, `SearchByOEM primary`, `PartsForVehicle`, `SearchByOEMIndex`). Every one should show `type=ref` (not `type=ALL`) against its expected index.

```bash
mysql --host=<tecdoc-mysql-host> --user=<user> --password --database=<db> \
      < scripts/diagnostics/tecdoc_diagnostic_full.sql \
      > tecdoc-baseline-$(date +%Y-%m-%d).txt
```

Runtime: ~2 seconds. Metadata-only + indexed-single-row lookups. Safe against a prod replica any time.

### Full historical audit

The one-time discovery queries (table sizes, HK prefix coverage, aftermarket-brand probes, corpus verification) already ran and their answers are recorded outside the repo. If you refresh the TecDoc dump and need the full baseline again, use git history:

```bash
git show 4646b3a:scripts/diagnostics/tecdoc_diagnostic_full.sql \
  > tecdoc_full_baseline.sql
mysql < tecdoc_full_baseline.sql > new-baseline-$(date +%Y-%m-%d).txt
```

### Postgres drill-down (owned_catalog)

```bash
PGHOST=<qa-pg-host> PGUSER=<user> PGDATABASE=parts_engine \
  psql -f scripts/diagnostics/owned_catalog_postgres.sql
```

### MySQL drill-downs (supersession, vehicle_fitment)

Only run these when the corresponding strategy is showing 0 hits and you need to root-cause it:

```bash
mysql --host=<tecdoc-mysql-host> --user=<user> --password --database=tecdoc \
      < scripts/diagnostics/supersession_mysql.sql
```

### Live SSE capture

```bash
# Terminal 1 — start the audit
pwsh scripts/audit/audit-quality.ps1 -Modes supersession -InputCorpus scripts/audit/corpus-1500-v2.csv

# Terminal 2 — stream the debug log while it runs
curl -N https://qa.ifritah.com/api/debug/logs > qa-supersession-debug.log
```