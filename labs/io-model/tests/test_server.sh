#!/usr/bin/env bash
# test_server.sh — smoke test for v0_blocking
# Usage: bash tests/test_server.sh  (run from labs/io-model/)

set -euo pipefail

PORT=8081
BINARY="./v0_blocking"
PASS=0
FAIL=0

log()  { echo "[test] $*"; }
ok()   { echo "[PASS] $*"; PASS=$((PASS+1)); }
fail() { echo "[FAIL] $*"; FAIL=$((FAIL+1)); }

cleanup() {
    if [[ -n "${SERVER_PID:-}" ]]; then
        kill "$SERVER_PID" 2>/dev/null || true
        wait "$SERVER_PID" 2>/dev/null || true
    fi
}
trap cleanup EXIT

# ── 1. Compile ──────────────────────────────────────────────────────────────
log "Compiling v0_blocking..."
if gcc -O2 -Wall -Wextra -g -o v0_blocking src/v0_blocking.c; then
    ok "Compilation succeeded"
else
    fail "Compilation failed"
    exit 1
fi

# ── 2. Start server on a custom port (override PORT define via env not possible,
#        so we patch with a quick sed to a temp file for testing) ─────────────
log "Building test binary on port $PORT..."
sed "s/#define PORT.*8080/#define PORT    $PORT/" src/v0_blocking.c > /tmp/v0_test.c
if gcc -O2 -o /tmp/v0_test /tmp/v0_test.c; then
    ok "Test binary (port $PORT) compiled"
else
    fail "Test binary compilation failed"
    exit 1
fi

log "Starting server in background (port $PORT)..."
/tmp/v0_test &
SERVER_PID=$!
sleep 0.5   # give server time to bind

if ! kill -0 "$SERVER_PID" 2>/dev/null; then
    fail "Server failed to start"
    exit 1
fi
ok "Server started (pid=$SERVER_PID)"

# ── 3. Make 3 requests and check response ───────────────────────────────────
for i in 1 2 3; do
    log "Request $i..."
    BODY=$(curl -s --max-time 3 "http://127.0.0.1:$PORT/" 2>/dev/null || echo "")
    if echo "$BODY" | grep -q "hello"; then
        ok "Request $i: response contains 'hello'"
    else
        fail "Request $i: expected 'hello', got: '$BODY'"
    fi
done

# ── 4. Verify server still running ──────────────────────────────────────────
if kill -0 "$SERVER_PID" 2>/dev/null; then
    ok "Server still running after 3 requests"
else
    fail "Server exited unexpectedly"
fi

# ── 5. Kill server ───────────────────────────────────────────────────────────
kill "$SERVER_PID" 2>/dev/null || true
wait "$SERVER_PID" 2>/dev/null || true
SERVER_PID=""
ok "Server stopped cleanly"

# ── Summary ─────────────────────────────────────────────────────────────────
echo ""
echo "Results: $PASS passed, $FAIL failed"
[[ $FAIL -eq 0 ]] && exit 0 || exit 1
