# Parts Engine — single-container runtime image.
#
# This image bundles PostgreSQL + the Go server + the built React frontend
# into ONE container so the whole app boots with `docker run -p 8080:8080`.
# The DB migrations in db/migrations/ auto-apply on first boot via the
# postgres:17-alpine entrypoint hook (/docker-entrypoint-initdb.d/).
#
# Layout at runtime:
#   /app/server                 — Go binary (foreground process)
#   /app/qa_gate                — Go binary (QA gate; optional invocation)
#   /app/import_legacy_cache    — Go binary (SQLite → Postgres bootstrap)
#   /app/frontend/dist          — Built React SPA
#   /app/data                   — Read-only seed data (hk_parts.db, vpic.lite.db, …)
#   /docker-entrypoint-initdb.d — SQL migrations (auto-run on first boot)
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
FROM golang:1.25-alpine AS builder
WORKDIR /src
ENV GOTOOLCHAIN=local CGO_ENABLED=0 GOOS=linux
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY db/ db/
RUN go build -ldflags="-s -w" -o /out/server ./cmd/server && \
    go build -ldflags="-s -w" -o /out/qa_gate ./cmd/qa_gate && \
    go build -ldflags="-s -w" -o /out/import_legacy_cache ./cmd/import_legacy_cache

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
COPY --from=frontend /app/frontend/dist /app/frontend/dist
COPY data/ /app/data/
COPY db/migrations/ /docker-entrypoint-initdb.d/
COPY container/entrypoint.sh /entrypoint.sh

RUN chmod +x /entrypoint.sh /app/server /app/qa_gate /app/import_legacy_cache

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

# 8080 for the HTTP API + SPA. 5432 for direct SQL access — remove this line
# to keep the DB purely internal.
EXPOSE 8080 5432

HEALTHCHECK --interval=30s --timeout=5s --retries=3 --start-period=30s \
    CMD wget -qO- http://127.0.0.1:8080/health >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/entrypoint.sh"]
