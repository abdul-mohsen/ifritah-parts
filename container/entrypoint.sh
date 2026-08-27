#!/bin/sh
# Parts Engine — single-container entrypoint.
#
# Boots the embedded Postgres via the official postgres:17-alpine
# `docker-entrypoint.sh` in the background (which handles initdb, user +
# database creation, and running any /docker-entrypoint-initdb.d/*.sql
# migrations on first boot), waits until Postgres accepts connections on
# 127.0.0.1:5432, then runs /app/server in the foreground so SIGTERM/SIGINT
# from `docker stop` reach it cleanly.
#
# If either the Postgres or the server process exits, the whole container
# exits — so orchestrators (docker, k8s, systemd) can restart it as a unit.
#
# Requires busybox `sh` (ships with postgres:17-alpine).

set -eu

# ── Config (all overridable via env) ─────────────────────
: "${PGHOST:=127.0.0.1}"
: "${PGPORT:=5432}"
: "${PGUSER:=parts}"
: "${PGDATABASE:=parts_engine}"

# Match the pg entrypoint's expected names too (initdb reads these).
: "${POSTGRES_USER:=${PGUSER}}"
: "${POSTGRES_DB:=${PGDATABASE}}"

: "${STARTUP_TIMEOUT_SEC:=90}"

log() { echo "[entrypoint] $*"; }

log "starting embedded PostgreSQL ..."
# The official image entrypoint expects to be PID 1; running it in a
# background subshell keeps signal handling on our side.
docker-entrypoint.sh postgres &
PG_PID=$!

log "waiting up to ${STARTUP_TIMEOUT_SEC}s for postgres on ${PGHOST}:${PGPORT} ..."
elapsed=0
until pg_isready -h "${PGHOST}" -p "${PGPORT}" -U "${PGUSER}" -d "${PGDATABASE}" >/dev/null 2>&1; do
    if ! kill -0 "${PG_PID}" 2>/dev/null; then
        log "postgres exited before becoming ready — see logs above"
        exit 1
    fi
    if [ "${elapsed}" -ge "${STARTUP_TIMEOUT_SEC}" ]; then
        log "timeout after ${STARTUP_TIMEOUT_SEC}s waiting for postgres"
        kill -TERM "${PG_PID}" 2>/dev/null || true
        exit 1
    fi
    sleep 1
    elapsed=$((elapsed + 1))
done
log "postgres ready after ${elapsed}s"

# Propagate container signals to both processes.
_shutdown() {
    log "shutdown signal — stopping server + postgres"
    if [ -n "${APP_PID:-}" ]; then
        kill -TERM "${APP_PID}" 2>/dev/null || true
    fi
    kill -TERM "${PG_PID}" 2>/dev/null || true
    wait "${PG_PID}" 2>/dev/null || true
    exit 0
}
trap _shutdown INT TERM

log "starting parts-engine server on ${BIND_ADDR:-0.0.0.0}:${PORT:-8080} ..."
/app/server &
APP_PID=$!

# ── First-boot auto-bootstrap ────────────────────────────
# When hk_parts_cache is empty, load the bundled SQLite dump
# (/app/data/hk_parts.db) into Postgres via import_legacy_cache. This
# fills hk_parts_cache + oem_search_index + vehicle_lookup + the
# platform/bridge/substitution tables in one shot so search actually
# returns real data on a fresh container.
#
# Runs in a background subshell so we don't block the server startup
# path. Idempotent — the row-count check skips on every boot after the
# first one, or when an operator has already loaded data.
(
    # Wait up to 60s for the server to apply migrations (creates the
    # target tables). Poll for hk_parts_cache existence rather than a
    # timer so we don't race a slow migration.
    i=0
    while [ "$i" -lt 30 ]; do
        if PGPASSWORD="${PGPASSWORD:-}" psql -h "${PGHOST}" -p "${PGPORT}" -U "${PGUSER}" -d "${PGDATABASE}" -Atc "SELECT 1 FROM hk_parts_cache LIMIT 1" >/dev/null 2>&1; then
            break
        fi
        i=$((i + 1))
        sleep 2
    done

    CACHE_COUNT=$(PGPASSWORD="${PGPASSWORD:-}" psql -h "${PGHOST}" -p "${PGPORT}" -U "${PGUSER}" -d "${PGDATABASE}" -Atc "SELECT COUNT(*) FROM hk_parts_cache" 2>/dev/null || echo 0)
    if [ "${CACHE_COUNT:-0}" -lt 100 ]; then
        log "bootstrap: hk_parts_cache has ${CACHE_COUNT} rows — importing from ${DATA_DIR:-/app/data}/hk_parts.db"
        if /app/import_legacy_cache 2>&1 | while IFS= read -r line; do log "  import: $line"; done; then
            log "bootstrap: import complete"
        else
            log "bootstrap: import_legacy_cache failed (non-fatal — server continues)"
        fi
    else
        log "bootstrap: hk_parts_cache has ${CACHE_COUNT} rows — skip"
    fi
) &

# Wait for the server. If it exits, tear down postgres and propagate the code.
wait "${APP_PID}"
APP_EXIT=$?

log "server exited with code ${APP_EXIT} — stopping postgres"
kill -TERM "${PG_PID}" 2>/dev/null || true
wait "${PG_PID}" 2>/dev/null || true

exit ${APP_EXIT}
