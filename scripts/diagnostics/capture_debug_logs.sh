#!/bin/bash
# ============================================================================
# capture_debug_logs.sh - stream qa /api/debug/logs SSE while an audit runs
# ============================================================================
# The qa deployment ships an SSE debug endpoint that streams every internal
# SQL error, slow-query warning, and strategy step. When a strategy returns
# 0 hits, the log line just before it usually says WHY (missing table,
# empty result set, ctx deadline, JOIN error).
#
# Usage:
#   # In terminal 1: start capturing (run for at least 60 seconds)
#   ./scripts/diagnostics/capture_debug_logs.sh > qa-debug-strategy.log &
#   CAPTURE_PID=$!
#
#   # In terminal 2: fire probe requests for the broken strategy
#   for MODE in supersession owned_catalog vin_assembly vehicle_fitment; do
#     for OEM in 26350-2J001 58101-3XA00 82460-2T010; do
#       curl -sk "https://qa.ifritah.com/api/search?q=$OEM&mode=$MODE&enrichmentLevel=none" > /dev/null
#     done
#   done
#
#   # After probes finish, stop the capture:
#   sleep 5 && kill $CAPTURE_PID
#
#   # Inspect the log:
#   grep -E '(SQL SLOW|SQL ERROR|STEP ERROR|REJECTED|articleId=0)' qa-debug-strategy.log
# ============================================================================

set -euo pipefail

ENDPOINT="${QA_ENDPOINT:-https://qa.ifritah.com}"
DURATION="${CAPTURE_DURATION:-120}"

echo "[capture_debug_logs] streaming ${ENDPOINT}/api/debug/logs for ${DURATION}s" >&2

# curl -N disables buffering — SSE frames land as they arrive.
# --max-time bounds the total capture so the script exits cleanly.
curl -sN --max-time "$DURATION" "${ENDPOINT}/api/debug/logs"
