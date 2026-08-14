# ── Stage 1: Build frontend ──────────────────────────────
FROM node:22-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit
COPY frontend/ ./
RUN npm run build

# ── Stage 2: Build Go server ────────────────────────────
FROM golang:latest AS builder
WORKDIR /app
ENV GOTOOLCHAIN=local
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /server ./cmd/server

# ── Stage 3: Final image ────────────────────────────────
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

COPY --from=builder /server ./server
COPY --from=frontend /app/frontend/dist ./frontend/dist
# Data directory is mounted at runtime via volume
RUN mkdir -p /app/data

EXPOSE 8080
ENV BIND_ADDR=0.0.0.0 \
    PORT=8080 \
    DATA_DIR=/app/data
# Deployers MUST set CORS_ORIGINS to an explicit allowlist (e.g.
# "https://ifritah.com,https://qa.ifritah.com"). We intentionally do NOT
# default to `*` here: the Gin CORS handler is configured with
# AllowCredentials=true, and the browser rejects that combination — worse,
# gin-contrib/cors echoes the request Origin, allowing any site to make
# credentialed requests.

ENTRYPOINT ["./server"]
