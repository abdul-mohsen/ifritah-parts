# M0 Broken-Strategy Diagnostics

Diagnostic SQL scripts you can run against qa (or any deployed env) to find why each broken strategy returns 0 hits. Results feed the M0 fix tasks in `docs/sprints/M0-fix-broken-strategies.md`.

## Files

| Task | Database | Script |
|---|---|---|
| M0.T1 owned_catalog | qa Postgres | `owned_catalog_postgres.sql` |
| M0.T2 supersession | qa MySQL (TecDoc) | `supersession_mysql.sql` |
| M0.T3 vin_assembly | qa Postgres + code trace | `vin_assembly_diagnosis.md` |
| M0.T4 vehicle_fitment | qa MySQL (TecDoc) + Postgres | `vehicle_fitment_mysql.sql` |
| All strategies | live SSE endpoint | `capture_debug_logs.sh` |

## How to run

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
