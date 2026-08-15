# `sql/` — archived legacy MySQL DDL (reference only)

These scripts are the **original MySQL side** of the schema. They were run
once against a MySQL TecDoc source to pre-compute the Hyundai/Kia parts
cache and populate `data/hk_parts.db` (the SQLite file the app still ships).

**Nothing in the running code reads these files.** They are kept as
historical reference for:

- Reconstructing `data/hk_parts.db` from a fresh MySQL TecDoc dump.
- Understanding the ETL that produced the current SQLite seed
  (`INSERT INTO hk_parts_cache SELECT ... FROM articlesvehicletrees ...`).
- Cross-checking that the new Postgres migrations under `db/migrations/`
  match the original MySQL schema table-for-table.

## Port map — legacy MySQL DDL → active Postgres migrations

| Legacy MySQL DDL (this folder) | Active Postgres migration (`db/migrations/`) | Port status |
| --- | --- | --- |
| `01_create_cache.sql` (`hk_parts_cache`) | `000001_create_hk_parts_cache.sql` | 1-to-1: 17 columns, 5 secondary indexes, same 3-column composite PK. Syntax delta: `INT AUTO_INCREMENT` → `BIGSERIAL`, `VARCHAR(N)` → `TEXT`, camelCase → snake_case, inline `INDEX` → separate `CREATE INDEX IF NOT EXISTS`, no `ENGINE=InnoDB`. |
| `02_fix_cache_brands.sql` | `000002_fix_cache_brands.sql` | 1-to-1 column additions. |
| `02_platform_map.sql` (`hk_platform_map`) | `000003_create_hk_platform_map.sql` | 1-to-1: 7 columns, 3 secondary indexes. |
| `03_nhtsa_bridge.sql` (`nhtsa_tecdoc_bridge`) | `000004_create_nhtsa_tecdoc_bridge.sql` | 1-to-1: 8 columns, 2 secondary indexes. |
| `04_oem_index.sql` (`oem_search_index`) | `000005_create_oem_search_index.sql` | 1-to-1: 9 columns, 3 secondary indexes. `ENUM(...)` → `TEXT NOT NULL CHECK (col IN (...))`. |
| `05_aftermarket_crossref.sql` (`aftermarket_crossref`) | *(no Postgres equivalent — table lives only in SQLite `hk_parts.db`)* | The Go code paths that use `aftermarket_crossref` (`internal/service/smart_search.go` `aftermarket_crossref_only` strategy, `enrichAftermarket`) query the **SQLite** `data/hk_parts.db` directly. The shipped SQLite already contains this table populated from RockAuto scrapes; no Postgres port is needed for the current runtime. |

## New migrations added on the Postgres side that had NO MySQL equivalent

These are net-new tables introduced by the Postgres migration set:

- `000006_create_vehicle_lookup.sql` — `vehicle_lookup` (flattened NHTSA ↔ linkage-target map used by offline mode).
- `000007_create_external_sources.sql` — `external_sources`, `external_source_assessments`, `external_part_refs`, `external_artifacts`, `external_install_hints` (source-registry governance).
- `000008_expand_oem_source_table_values.sql` — expands the `source_table` CHECK to include `discovered` and `substitution`.
- `000009_create_substitution_links.sql` — `substitution_links` (cautious source-backed replacement graph).
- `000010_create_commons_media_reviews.sql` — `commons_media_reviews` (internal manual-review queue for CC0/CC BY 4.0 illustrations).

## Reproducing the SQLite cache from a fresh MySQL TecDoc dump

If you ever need to rebuild `data/hk_parts.db` from a fresh MySQL TecDoc
source, the historical procedure is documented step-by-step in this folder:

```
01_create_cache.sql          -- CREATE + ETL into hk_parts_cache
02_fix_cache_brands.sql      -- Backfill brand/category metadata
02_platform_map.sql          -- CREATE hk_platform_map + seed
03_nhtsa_bridge.sql          -- CREATE nhtsa_tecdoc_bridge + seed
04_oem_index.sql             -- CREATE oem_search_index + seed
05_aftermarket_crossref.sql  -- CREATE aftermarket_crossref in SQLite
```

Then export from MySQL to SQLite using the historical `cmd/export`
(dropped from `main` because it referenced the now-removed MySQL config
surface — recover it from `git show origin/main:cmd/export/main.go`
against a temporary MySQL-aware config if you need to run it again).
