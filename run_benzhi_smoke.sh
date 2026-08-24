#!/usr/bin/env bash
# DairyGate smoke test: build the server, start it on a local port backed by a
# temporary SQLite database, exercise the real HTTP API (health, task build,
# snapshot read), then clean up every process and temporary file.
#
# The script is deterministic and performs no external network access. It is
# intentionally NOT a wrapper around `go test`: it drives the compiled binary
# over HTTP exactly as an operator would.
set -euo pipefail

# Resolve to the repository root (the directory containing this script).
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"

BIN="$(mktemp -t dairygate-bin.XXXXXX)"
DB="$(mktemp -t dairygate-db.XXXXXX)"
LOG="$(mktemp -t dairygate-log.XXXXXX)"
PID=""
# Bind to an ephemeral port so the smoke test never collides with a process
# already listening on a well-known port; the server reports the real address.
ADDR="${DAIRYGATE_ADDR:-127.0.0.1:0}"

cleanup() {
  if [[ -n "$PID" ]] && kill -0 "$PID" 2>/dev/null; then
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
  rm -f "$BIN" "$DB" "${DB}-wal" "${DB}-shm" "$LOG"
}
trap cleanup EXIT

echo "==> building frontend assets"
# Rebuild the embedded console so a fresh checkout (without webembed/dist)
# still compiles. The build is a pure local copy with no network access.
node web/build.mjs

echo "==> building dairygate"
go build -o "$BIN" ./cmd/dairygate

echo "==> starting server (db=${DB})"
"$BIN" -addr "$ADDR" -db "$DB" >"$LOG" 2>&1 &
PID=$!

# Wait for the server to report its bound address, then probe readiness.
ADDR_LINE=""
for _ in $(seq 1 50); do
  if ADDR_LINE="$(grep -m1 'LISTEN_ADDR=' "$LOG" 2>/dev/null)"; then
    break
  fi
  sleep 0.1
done
if [[ -z "$ADDR_LINE" ]]; then
  echo "server did not report a listen address" >&2
  cat "$LOG" >&2
  exit 1
fi
LISTEN_ADDR="${ADDR_LINE##*LISTEN_ADDR=}"
PORT="${LISTEN_ADDR##*:}"
BASE="http://127.0.0.1:${PORT}"
echo "    server listening on ${BASE}"

# Wait for the health endpoint to become ready (bounded, no external network).
ready=0
for _ in $(seq 1 50); do
  if resp="$(curl -sS --max-time 1 "${BASE}/api/health" 2>/dev/null)"; then
    ready=1
    break
  fi
  sleep 0.1
done
if [[ "$ready" != "1" ]]; then
  echo "server did not become ready" >&2
  cat "$LOG" >&2
  exit 1
fi

echo "==> probing health"
health="$(curl -sS "${BASE}/api/health")"
if [[ "$health" != *'"status":"ok"'* ]]; then
  echo "unexpected health response: $health" >&2
  exit 1
fi
echo "    health: $health"

echo "==> building a tank-batch inspection task"
create_body='{"taskId":"smoke-task","farmId":"farm-dairy-001","tankBatch":"BATCH-SMOKE","compartments":["A","B"],"seals":["seal-0001","seal-0002"],"recorderModel":"recorder-x1","ruleVersion":"rules-v2026-1","reviewers":["person-reviewer-a","person-reviewer-b"]}'
create_resp="$(curl -sS -X POST "${BASE}/api/tasks" -H 'Content-Type: application/json' -d "$create_body")"
if [[ "$create_resp" != *'"status":"pending_sampling"'* ]]; then
  echo "unexpected create response: $create_resp" >&2
  exit 1
fi
echo "    create: $create_resp"

echo "==> reading the task snapshot"
snapshot="$(curl -sS "${BASE}/api/tasks/smoke-task")"
if [[ "$snapshot" != *'"taskId":"smoke-task"'* ]] || [[ "$snapshot" != *'"tankBatch":"BATCH-SMOKE"'* ]]; then
  echo "unexpected snapshot response: $snapshot" >&2
  exit 1
fi
echo "    snapshot: $snapshot"

echo "==> verifying rejection of a duplicate build"
dup_resp="$(curl -sS -X POST "${BASE}/api/tasks" -H 'Content-Type: application/json' -d "$create_body")"
if [[ "$dup_resp" != *'"error"'* ]]; then
  echo "expected a conflict error, got: $dup_resp" >&2
  exit 1
fi
echo "    duplicate build rejected: $dup_resp"

echo "==> smoke test passed"
