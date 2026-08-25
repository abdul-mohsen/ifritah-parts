# M0 Broken-Strategy Diagnostics + TecDoc Health

Diagnostic SQL scripts you can run against qa (or any deployed env) to find why each broken strategy returns 0 hits, plus the ONE all-in-one TecDoc-health report that answers "what data actually exists in my MySQL". Results feed the M0 fix tasks in `docs/sprints/M0-fix-broken-strategies.md`.

## Files

| Task | Database | Script |
|---|---|---|
| **TecDoc DB health (start here)** | **qa MySQL (TecDoc)** | **`tecdoc_health_report.sql`** |
| M0.T1 owned_catalog | qa Postgres | `owned_catalog_postgres.sql` |
| M0.T2 supersession | qa MySQL (TecDoc) | `supersession_mysql.sql` |
| M0.T3 vin_assembly | qa Postgres + code trace | `vin_assembly_diagnosis.md` |
| M0.T4 vehicle_fitment | qa MySQL (TecDoc) + Postgres | `vehicle_fitment_mysql.sql` |
| All strategies | live SSE endpoint | `capture_debug_logs.sh` |

## How to run

### `tecdoc_health_report.sql` — the big one, run this first

Single self-contained report covering all 13 sections: table sizes, index presence (sql/06 + sql/07), HK coverage per table, language distribution, test-corpus verification, sample rows, and query-plan EXPLAINs.

```bash
mysql --host=<tecdoc-mysql-host> \
      --user=<user> --password \
      --database=<tecdoc-db-name> \
      < scripts/diagnostics/tecdoc_health_report.sql \
      > tecdoc-health-$(date +%Y-%m-%d).txt
```

Or interactively:

```
mysql> source scripts/diagnostics/tecdoc_health_report.sql;
```

Runtime: 2-15 min depending on whether sql/06 and sql/07 indexes are in place. Read-only, safe against a live prod replica. Each section prints a header banner so the output is easy to scan.

Paste the full output back to me and I'll diagnose:

- whether the pending sql/06 + sql/07 migrations still need to run
- how much of the audit corpus is actually reachable
- whether language / linkingTargetType filters are eliminating data
- where the biggest coverage gaps are per HK category

### Postgres queries (owned_catalog, catalog wiring)

```bash
# From a machine with psql + Postgres creds:
PGHOST=<qa-pg-host> PGUSER=<user> PGDATABASE=parts_engine \
  psql -f scripts/diagnostics/owned_catalog_postgres.sql

# Or from inside the qa container:
docker exec -it parts-engine psql -U postgres -d parts_engine \
  -f /app/scripts/diagnostics/owned_catalog_postgres.sql
```

### MySQL queries (supersession, vehicle_fitment)

The TecDoc MySQL is a managed instance — you need the read-replica credentials.

```bash
mysql --host=<tecdoc-mysql-host> \
      --user=<user> --password \
      --database=tecdoc \
      -e "$(cat scripts/diagnostics/supersession_mysql.sql)"
```

### Live SSE debug capture (any strategy)

```bash
# Start the audit run first in another terminal:
pwsh scripts/audit/audit-quality.ps1 -Modes supersession -InputCorpus scripts/audit/corpus-1500-v2.csv

# In parallel, stream the debug log:
curl -N https://qa.ifritah.com/api/debug/logs > qa-supersession-debug.log
```

## Deliverable per task

For each diagnostic run, paste the output into a new file under
`docs/data-sources/<task>-diagnosis.md` and open a PR that references
the M0 task ID. The fix PR will follow, informed by the finding.
