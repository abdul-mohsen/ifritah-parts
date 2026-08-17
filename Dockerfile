# Parts Engine — single-container runtime image.
#
# This image bundles PostgreSQL + the Go server + the built React frontend
# into ONE container so the whole app boots with `docker run -p 8080:8080`.
#
# Schema migrations are applied by the app itself on every start via
# internal/db/migrator.go — the SQL files are embedded into the binary
# through db/migrations/embed.go, so nothing extra needs to be shipped.
# This replaces the previous /docker-entrypoint-initdb.d/ approach which
# only ran migrations on first boot (empty volume) and couldn't handle
# migrations added after the initial deploy.
#
# The migration runner uses a Postgres advisory lock so multi-replica
# deployments (docker-compose scale, k8s multi-pod) never race, and every
# migration file uses CREATE TABLE IF NOT EXISTS + ON CONFLICT DO NOTHING
# so re-running is a no-op.
#
# The TecDoc-derive worker also starts automatically (see main.go). It
# no-ops when TECDOC_* env vars aren't set — the hand-curated seed
# baseline in migration 000011 remains active.
#
# Layout at runtime:
#   /app/server                 — Go binary (foreground process; runs
#                                 migrations + starts derive worker on boot)
#   /app/qa_gate                — Go binary (QA gate; optional invocation)
#   /app/import_legacy_cache    — Go binary (SQLite → Postgres bootstrap)
#   /app/derive_hk_maps         — Go binary (CLI wrapper — normally not
#                                 needed since main.go auto-runs derive;
#                                 kept for on-demand --force runs)
#   /app/frontend/dist          — Built React SPA
#   /app/data                   — Read-only seed data (hk_parts.db, vpic.lite.db, …)
#   /var/lib/postgresql/data    — Postgres data (mount a volume for persistence)
#   /entrypoint.sh              — Boots pg → waits → starts server
#
# For a proper multi-container setup (separate postgres), see docker-compose.yml
# — the single-container image still runs fine in that mode; postgres just
# double-runs harmlessly.

# ── Stage 1: Frontend build ─────────────────────────────
FROM node:22-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit
COPY frontend/ ./
RUN npm run build

# ── Stage 2: Go server build ────────────────────────────
# Use the Debian-based golang image (not -alpine) for two reasons:
#   1. The full CA bundle is preinstalled, so `go mod download` succeeds in
#      environments where Alpine's smaller trust store is intercepted by a
#      corporate SSL proxy (observed on the operator's local Docker Desktop
#      when the pre-merge Dockerfile used golang:1.25-alpine).
#   2. GOPROXY defaults to https://proxy.golang.org — set GOPROXY=direct as a
#      belt-and-braces fallback so restrictive networks that block proxy.golang.org
#      still resolve modules from source.
FROM golang:1.25 AS builder
WORKDIR /src
ENV CGO_ENABLED=0 GOOS=linux
COPY go.mod go.sum ./
# --mount=type=cache reuses the module cache across builds when BuildKit is on.
# `|| GOPROXY=direct go mod download` is a fallback for networks that block
# proxy.golang.org.
RUN go mod download || (GOPROXY=direct go mod download)
COPY cmd/ cmd/
COPY internal/ internal/
COPY scripts/ scripts/
COPY db/ db/
RUN go build -ldflags="-s -w" -o /out/server ./cmd/server && \
    go build -ldflags="-s -w" -o /out/qa_gate ./cmd/qa_gate && \
    go build -ldflags="-s -w" -o /out/import_legacy_cache ./cmd/import_legacy_cache && \
    go build -ldflags="-s -w" -o /out/derive_hk_maps ./scripts/derive_hk_maps

# ── Stage 3: Runtime — Postgres + Server + SPA ──────────
FROM postgres:17-alpine

# postgres:17-alpine ships with:
#   - ca-certificates.crt at /etc/ssl/certs/ (for outbound HTTPS to NHTSA)
#   - tzdata pre-installed
#   - busybox wget at /usr/bin/wget (used for HEALTHCHECK)
# No apk add needed.

WORKDIR /app

COPY --from=builder /out/server /app/server
COPY --from=builder /out/qa_gate /app/qa_gate
COPY --from=builder /out/import_legacy_cache /app/import_legacy_cache
COPY --from=builder /out/derive_hk_maps /app/derive_hk_maps
COPY --from=frontend /app/frontend/dist /app/frontend/dist
COPY data/ /app/data/
COPY container/entrypoint.sh /entrypoint.sh

# NOTE: migrations are NOT copied to /docker-entrypoint-initdb.d/ any more.
# The app-managed migrator in internal/db/migrator.go runs them on every
# boot from the SQL files embedded into /app/server via db/migrations/embed.go.
# This handles both the first-boot bootstrap AND any migrations added later.

RUN chmod +x /entrypoint.sh /app/server /app/qa_gate /app/import_legacy_cache /app/derive_hk_maps

# ── Embedded Postgres defaults ────────────────────────────
# These names must match the app's PG* env vars below. The postgres:17-alpine
# entrypoint uses POSTGRES_USER / POSTGRES_DB / POSTGRES_PASSWORD to run
# initdb + createuser on FIRST boot. Later boots reuse the volume as-is.
ENV POSTGRES_USER=parts \
    POSTGRES_DB=parts_engine \
    POSTGRES_PASSWORD=parts_engine_pw \
    PGDATA=/var/lib/postgresql/data

# ── App defaults ─────────────────────────────────────────
# App connects to the embedded postgres on 127.0.0.1:5432 (in-container).
# CORS_ORIGINS is intentionally left unset — deployers must set an explicit
# allowlist (e.g. "https://ifritah.com,https://qa.ifritah.com"). The server
# refuses to enable AllowCredentials when the allowlist contains "*".
#
# MYSQL_*  — optional. When set, the TecDoc reader + the derive worker
#            connect to an external TecDoc MySQL instance. When unset, the
#            app runs with prefix-inference seed baseline only. This is the
#            fully-decoupled operating mode.
ENV BIND_ADDR=0.0.0.0 \
    PORT=8080 \
    DATA_DIR=/app/data \
    FRONTEND_DIR=/app/frontend/dist \
    PGHOST=127.0.0.1 \
    PGPORT=5432 \
    PGUSER=parts \
    PGPASSWORD=parts_engine_pw \
    PGDATABASE=parts_engine \
    PGSSLMODE=disable \
    NHTSA_URL=https://vpic.nhtsa.dot.gov/api \
    NHTSA_RECALLS_URL=https://api.nhtsa.gov/recalls

# 8080 for the HTTP API + SPA. The container ALWAYS listens on 8080; the
# host-side port is picked at run time (docker-compose reads $PORT from .env
# and maps HOST:$PORT → CONTAINER:8080). 5432 exposes the embedded postgres
# for direct SQL access — remove this line to keep the DB purely internal.
EXPOSE 8080 5432

HEALTHCHECK --interval=30s --timeout=5s --retries=3 --start-period=30s \
    CMD wget -qO- http://127.0.0.1:8080/health >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/entrypoint.sh"]
