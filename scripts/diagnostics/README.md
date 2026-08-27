# M0 Broken-Strategy Diagnostics + TecDoc Health

Diagnostic SQL scripts you can run against qa (or any deployed env) to find why each broken strategy returns 0 hits, plus the ONE all-in-one TecDoc-health report that answers "what data actually exists in my MySQL". Results feed the M0 fix tasks in `docs/sprints/M0-fix-broken-strategies.md`.

> **Prefer one-command engine-health check?** See
> [`scripts/engine-health-check.md`](../engine-health-check.md) — orchestrates
> the API quality audit + this diagnostic + a delta comparison into a single
> combined report at `docs/reports/{date}-engine-health/summary.md`.

## Files

| Task | Database | Script |
|---|---|---|
| **TecDoc DB audit + diagnostic (start here)** | **qa MySQL / MariaDB (TecDoc)** | **`tecdoc_diagnostic_full.sql`** |
| M0.T1 owned_catalog | qa Postgres | `owned_catalog_postgres.sql` |
| M0.T2 supersession (drill-down) | qa MySQL (TecDoc) | `supersession_mysql.sql` |
| M0.T3 vin_assembly | qa Postgres + code trace | `vin_assembly_diagnosis.md` |
| M0.T4 vehicle_fitment (drill-down) | qa MySQL (TecDoc) + Postgres | `vehicle_fitment_mysql.sql` |
| All strategies (live capture) | live SSE endpoint | `capture_debug_logs.sh` |

## How to run

### `tecdoc_diagnostic_full.sql` — the ONE big audit + diagnostic

Single self-contained file. Runs the whole TecDoc health check end-to-end in 6 parts (23 numbered sections total). Answers every question that gates M0-M8 progress without needing to chain multiple scripts together.

**What it covers**

- **Part A · Environment** — table sizes, schema, P0 index PASS/FAIL summary (sql/06 + sql/07 + sql/08 hotfix), articles/ambrand/articlecriteria column discovery, HK manufacturers + brand catalog
- **Part B · HK data coverage** — oem_number, articlecrosses, articlecriteria, articlesvehicletrees linkingTargetType, supersession, vehicle catalog, language distribution
- **Part C · The REAL aftermarket answer** — per-HK-prefix aftermarket brands via `articles.dataSupplierId → ambrand.brandId` (the correct JOIN, not `mfrId`) + explicit marquee-brand probe (BOSCH / MANN / MAHLE / DENSO / VALEO / HELLA / BREMBO / TEXTAR / FEBI / LEMFOERDER / LuK / INA / SKF / GATES / ContiTech / etc.)
- **Part D · Corpus verification** — the 19 real HK OEMs from `scripts/audit/corpus-1500-v2.csv`: does each resolve? what aftermarket brands surface per OEM? what specs are populated?
- **Part E · Sample rows** — spot-check the data shape
- **Part F · EXPLAIN plans** — validates every index is actually used by the query planner

**Run it**

```bash
mysql --host=<tecdoc-mysql-host> \
      --user=<user> --password \
      --database=<tecdoc-db-name> \
      < scripts/diagnostics/tecdoc_diagnostic_full.sql \
      > tecdoc-diagnostic-$(date +%Y-%m-%d).txt
```

Or interactively:

```
mysql> source scripts/diagnostics/tecdoc_diagnostic_full.sql;
```

**Runtime characteristics**

| | |
|---|---|
| Target runtime | 2-8 minutes on a healthy DB (all P0 indexes applied) |
| Longer when | sql/07 or sql/08 indexes missing — EXPLAINs in Part F surface which one |
| Memory-safe | No `COUNT(DISTINCT)` over 340M rows; sampled estimates where full scans would fill /var/tmp |
| Compatibility | MariaDB 10.3+ **AND** MySQL 5.7 / 8.x — no window functions, no `FORMAT=TREE`, no `JSON_TABLE`, no CTEs, no reserved-word column aliases |
| Side effects | None. Every temp table is `TEMPORARY ... ENGINE=MEMORY` (session-scoped, auto-drop) |
| Reads only | No `INSERT/UPDATE/DELETE` against user tables. No DDL against user tables. |

Paste the full output back and I'll diagnose section-by-section:
- whether sql/06 / sql/07 / sql/08 migrations are all applied (Part A §2c pass/fail summary)
- whether the 19 audit-corpus OEMs actually resolve to data (Part D §15)
- what REAL aftermarket brands exist for HK OEMs (Part C §12 + Part D §16b/c)
- whether the language + `linkingTargetType='P'` filters eliminate data (Part B §8 + §11)
- whether every hot query hits an index (Part F §18-§23)

### Postgres queries (owned_catalog, catalog wiring)

```bash
# From a machine with psql + Postgres creds:
PGHOST=<qa-pg-host> PGUSER=<user> PGDATABASE=parts_engine \
  psql -f scripts/diagnostics/owned_catalog_postgres.sql

# Or from inside the qa container:
docker exec -it parts-engine psql -U postgres -d parts_engine \
  -f /app/scripts/diagnostics/owned_catalog_postgres.sql
```

### MySQL drill-down queries (supersession, vehicle_fitment)

The TecDoc MySQL is a managed instance — you need the read-replica credentials.

```bash
mysql --host=<tecdoc-mysql-host> \
      --user=<user> --password \
      --database=tecdoc \
      < scripts/diagnostics/supersession_mysql.sql
```

These are narrower drill-downs used when `tecdoc_diagnostic_full.sql` surfaces a specific issue in Part B §9 or the vehicle-fitment sections. Most of the time you won't need them — the consolidated report has enough coverage.

### Live SSE debug capture (any strategy)

```bash
# Start the audit run first in another terminal:
pwsh scripts/audit/audit-quality.ps1 -Modes supersession -InputCorpus scripts/audit/corpus-1500-v2.csv

# In parallel, stream the debug log:
curl -N https://qa.ifritah.com/api/debug/logs > qa-supersession-debug.log
```

## Deliverable per task

For each diagnostic run, paste the output into a new file under `docs/data-sources/<task>-diagnosis.md` and open a PR that references the M0 task ID. The fix PR will follow, informed by the finding.
