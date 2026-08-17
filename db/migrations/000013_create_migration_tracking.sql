-- ============================================================================
-- Migration tracking + background task tracking
-- ============================================================================
-- Runs in EVERY container boot via the app-managed migrator (see
-- internal/db/migrator.go). Idempotent — CREATE TABLE IF NOT EXISTS + safe
-- re-inserts. This table is what allows the app to skip already-applied
-- migrations across restarts.
--
-- background_task_runs tracks scheduled work (currently: TecDoc-derive of
-- hk_oem_prefix_map + hk_chassis_code_map) so the app only re-runs it when
-- it's actually time (default cadence: every 30 days).
-- ============================================================================

CREATE TABLE IF NOT EXISTS schema_migrations (
    version      TEXT PRIMARY KEY,        -- e.g. '000012_create_oem_resolution_cache'
    applied_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    applied_by   TEXT NOT NULL DEFAULT 'app_migrator'
);

CREATE TABLE IF NOT EXISTS background_task_runs (
    task_key      TEXT PRIMARY KEY,       -- 'derive_hk_maps', 'sitemap_ingest', ...
    last_run_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_status   TEXT NOT NULL,          -- 'success' | 'error' | 'skipped'
    last_message  TEXT NOT NULL DEFAULT '',
    next_run_at   TIMESTAMPTZ             -- when the scheduler may re-fire
);

COMMENT ON TABLE schema_migrations    IS 'App-managed migration tracker — see internal/db/migrator.go';
COMMENT ON TABLE background_task_runs IS 'Scheduled background tasks (TecDoc derive, sitemap ingest, …). Persistent + double-run-safe.';
