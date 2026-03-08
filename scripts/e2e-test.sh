#!/usr/bin/env bash
set -euo pipefail

# E2E test for devlb: init → start → route → exec → switch → stop
# Usage: ./scripts/e2e-test.sh
# Requires: python3, curl

DEVLB="${DEVLB:-./devlb}"
LISTEN_PORT=18900
LISTEN_PORT2=18901
PASS=0
FAIL=0
PIDS_TO_KILL=()

cleanup() {
    for pid in "${PIDS_TO_KILL[@]}"; do
        kill "$pid" 2>/dev/null || true
        wait "$pid" 2>/dev/null || true
    done
    "$DEVLB" stop >/dev/null 2>&1 || true
    rm -rf "$TMPDIR_E2E" 2>/dev/null || true
}
trap cleanup EXIT

TMPDIR_E2E=$(mktemp -d)
export HOME="$TMPDIR_E2E"

assert_eq() {
    local label="$1" expected="$2" actual="$3"
    if [ "$expected" = "$actual" ]; then
        echo "  PASS: $label"
        PASS=$((PASS + 1))
    else
        echo "  FAIL: $label (expected='$expected', actual='$actual')"
        FAIL=$((FAIL + 1))
    fi
}

assert_contains() {
    local label="$1" pattern="$2" text="$3"
    if echo "$text" | grep -q "$pattern"; then
        echo "  PASS: $label"
        PASS=$((PASS + 1))
    else
        echo "  FAIL: $label (pattern='$pattern' not found in output)"
        FAIL=$((FAIL + 1))
    fi
}

assert_http() {
    local label="$1" url="$2" expected_code="$3"
    local code
    code=$(curl -s -o /dev/null -w "%{http_code}" "$url" 2>/dev/null || echo "000")
    assert_eq "$label" "$expected_code" "$code"
}

# ── Build ──
echo "▶ Building devlb..."
go build -o "$DEVLB" ./cmd/devlb/
echo ""

# ── 1. Init ──
echo "▶ Test: init"
"$DEVLB" init >/dev/null 2>&1

# Overwrite config with test ports
cat > "$TMPDIR_E2E/.devlb/devlb.yaml" <<EOF
services:
  - name: api
    port: $LISTEN_PORT
  - name: auth
    port: $LISTEN_PORT2
EOF
assert_eq "config created" "0" "$?"
echo ""

# ── 2. Start ──
echo "▶ Test: start"
"$DEVLB" start >/dev/null 2>&1
sleep 0.5
output=$("$DEVLB" status 2>&1)
assert_contains "status shows ports" ":$LISTEN_PORT" "$output"
assert_contains "status shows idle" "idle" "$output"
echo ""

# ── 3. Route / proxy / unroute ──
echo "▶ Test: route → proxy → unroute"
python3 -m http.server 18910 --bind 127.0.0.1 &>/dev/null &
PIDS_TO_KILL+=($!)
sleep 0.5

"$DEVLB" route $LISTEN_PORT 18910 --label manual-test >/dev/null 2>&1
output=$("$DEVLB" status 2>&1)
assert_contains "backend registered" "18910" "$output"
assert_contains "label shown" "manual-test" "$output"
assert_contains "active status" "active" "$output"

assert_http "proxy HTTP 200" "http://127.0.0.1:$LISTEN_PORT/" "200"

"$DEVLB" unroute $LISTEN_PORT 18910 >/dev/null 2>&1
output=$("$DEVLB" status 2>&1)
assert_contains "backend removed" "idle" "$output"

kill "${PIDS_TO_KILL[-1]}" 2>/dev/null; wait "${PIDS_TO_KILL[-1]}" 2>/dev/null || true
unset 'PIDS_TO_KILL[-1]'
echo ""

# ── 4. Exec (ptrace port interception) ──
echo "▶ Test: exec (ptrace)"
timeout 8 "$DEVLB" exec $LISTEN_PORT -- python3 -m http.server $LISTEN_PORT --bind 127.0.0.1 &>/dev/null &
EXEC_PID=$!
PIDS_TO_KILL+=($EXEC_PID)
sleep 3

output=$("$DEVLB" status 2>&1)
assert_contains "exec auto-registered" "active" "$output"
assert_contains "label auto-detected" "main\|unknown\|detached" "$output"

assert_http "exec proxy HTTP 200" "http://127.0.0.1:$LISTEN_PORT/" "200"

kill "$EXEC_PID" 2>/dev/null; wait "$EXEC_PID" 2>/dev/null || true
unset 'PIDS_TO_KILL[-1]'
sleep 0.5

output=$("$DEVLB" status 2>&1)
assert_contains "exec cleanup (idle after exit)" "idle" "$output"
echo ""

# ── 4b. Exec multi-port (ptrace port interception) ──
echo "▶ Test: exec multi-port (ptrace)"
# Build bindtest binary for multi-port bind
BINDTEST="$TMPDIR_E2E/bindtest"
go build -o "$BINDTEST" ./internal/portswap/testdata/bindtest/

timeout 8 "$DEVLB" exec $LISTEN_PORT,$LISTEN_PORT2 -- "$BINDTEST" "$LISTEN_PORT,$LISTEN_PORT2" --serve &>/dev/null &
EXEC_PID=$!
PIDS_TO_KILL+=($EXEC_PID)
sleep 3

output=$("$DEVLB" status 2>&1)
assert_contains "exec multi-port auto-registered (port1)" ":$LISTEN_PORT" "$output"
assert_contains "exec multi-port auto-registered (port2)" ":$LISTEN_PORT2" "$output"

kill "$EXEC_PID" 2>/dev/null; wait "$EXEC_PID" 2>/dev/null || true
unset 'PIDS_TO_KILL[-1]'
sleep 0.5

output=$("$DEVLB" status 2>&1)
assert_contains "exec multi-port cleanup (idle after exit)" "idle" "$output"
echo ""

# ── 5. Switch ──
echo "▶ Test: switch"
python3 -m http.server 18920 --bind 127.0.0.1 &>/dev/null &
PIDS_TO_KILL+=($!)
python3 -m http.server 18921 --bind 127.0.0.1 &>/dev/null &
PIDS_TO_KILL+=($!)
sleep 0.5

"$DEVLB" route $LISTEN_PORT 18920 --label worktree-a >/dev/null 2>&1
"$DEVLB" route $LISTEN_PORT 18921 --label worktree-b >/dev/null 2>&1

output=$("$DEVLB" status 2>&1)
assert_contains "two backends registered" "worktree-b" "$output"
assert_contains "first is active" "active" "$output"
assert_contains "second is standby" "standby" "$output"

assert_http "worktree-a proxy" "http://127.0.0.1:$LISTEN_PORT/" "200"

"$DEVLB" switch worktree-b >/dev/null 2>&1
output=$("$DEVLB" status 2>&1)
# worktree-b should now be active
assert_contains "worktree-b active after switch" "worktree-b.*active" "$output"

assert_http "worktree-b proxy" "http://127.0.0.1:$LISTEN_PORT/" "200"

"$DEVLB" unroute $LISTEN_PORT 18920 >/dev/null 2>&1
"$DEVLB" unroute $LISTEN_PORT 18921 >/dev/null 2>&1

for pid in "${PIDS_TO_KILL[@]}"; do
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
done
PIDS_TO_KILL=()
echo ""

# ── 6. HTTP 503 (no backend) ──
echo "▶ Test: HTTP 503 error page"
# No backend is routed — should get 503 for HTTP requests
code=$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:$LISTEN_PORT/" 2>/dev/null || echo "000")
assert_eq "503 when no backend" "503" "$code"

body=$(curl -s "http://127.0.0.1:$LISTEN_PORT/" 2>/dev/null || echo "")
assert_contains "503 body contains service info" "503" "$body"

# Route a backend on a dead port — should also get 503
"$DEVLB" route $LISTEN_PORT 18999 --label dead-backend >/dev/null 2>&1
code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 5 "http://127.0.0.1:$LISTEN_PORT/" 2>/dev/null || echo "000")
assert_eq "503 when backend is dead" "503" "$code"

"$DEVLB" unroute $LISTEN_PORT 18999 >/dev/null 2>&1
echo ""

# ── 7. Health check + failover ──
echo "▶ Test: health check failover"

# Stop daemon and restart with health check enabled
"$DEVLB" stop >/dev/null 2>&1
sleep 0.5

cat > "$TMPDIR_E2E/.devlb/devlb.yaml" <<EOF
services:
  - name: api
    port: $LISTEN_PORT
  - name: auth
    port: $LISTEN_PORT2
health_check:
  enabled: true
  interval: "500ms"
  timeout: "200ms"
  unhealthy_after: 2
EOF

"$DEVLB" start >/dev/null 2>&1
sleep 0.5

# Start two backends
python3 -m http.server 18930 --bind 127.0.0.1 &>/dev/null &
B1_PID=$!
PIDS_TO_KILL+=($B1_PID)
python3 -m http.server 18931 --bind 127.0.0.1 &>/dev/null &
B2_PID=$!
PIDS_TO_KILL+=($B2_PID)
sleep 0.5

"$DEVLB" route $LISTEN_PORT 18930 --label backend-a >/dev/null 2>&1
"$DEVLB" route $LISTEN_PORT 18931 --label backend-b >/dev/null 2>&1
sleep 0.5

# Verify both work
assert_http "health check: initial proxy works" "http://127.0.0.1:$LISTEN_PORT/" "200"

# Kill the active backend (backend-a)
kill "$B1_PID" 2>/dev/null; wait "$B1_PID" 2>/dev/null || true
unset 'PIDS_TO_KILL[-1]'
unset 'PIDS_TO_KILL[-1]'
PIDS_TO_KILL+=($B2_PID)

# Wait for health check to detect failure (2 checks * 500ms + margin)
sleep 2

# Should failover to backend-b
assert_http "health check: failover to healthy backend" "http://127.0.0.1:$LISTEN_PORT/" "200"

# Status should show unhealthy
output=$("$DEVLB" status 2>&1)
assert_contains "health check: unhealthy shown" "unhealthy" "$output"

"$DEVLB" unroute $LISTEN_PORT 18930 >/dev/null 2>&1
"$DEVLB" unroute $LISTEN_PORT 18931 >/dev/null 2>&1

kill "$B2_PID" 2>/dev/null; wait "$B2_PID" 2>/dev/null || true
PIDS_TO_KILL=()
echo ""

# ── 8. Config hot reload ──
echo "▶ Test: config hot reload"

output=$("$DEVLB" status 2>&1)
assert_contains "hot reload: initial has api port" ":$LISTEN_PORT" "$output"

# Add a new service via config file
cat > "$TMPDIR_E2E/.devlb/devlb.yaml" <<EOF
services:
  - name: api
    port: $LISTEN_PORT
  - name: auth
    port: $LISTEN_PORT2
  - name: web
    port: 18950
health_check:
  enabled: true
  interval: "500ms"
  timeout: "200ms"
  unhealthy_after: 2
EOF

# Wait for watcher to pick up change (2s poll interval + margin)
sleep 3

output=$("$DEVLB" status 2>&1)
assert_contains "hot reload: new service 'web' appeared" "18950" "$output"

# Remove auth service
cat > "$TMPDIR_E2E/.devlb/devlb.yaml" <<EOF
services:
  - name: api
    port: $LISTEN_PORT
  - name: web
    port: 18950
health_check:
  enabled: true
  interval: "500ms"
  timeout: "200ms"
  unhealthy_after: 2
EOF

sleep 3

output=$("$DEVLB" status 2>&1)
assert_contains "hot reload: 'web' still present" "18950" "$output"

# auth should be gone — check it does NOT appear
if echo "$output" | grep -q ":$LISTEN_PORT2"; then
    echo "  FAIL: hot reload: auth should be removed (port $LISTEN_PORT2 still present)"
    FAIL=$((FAIL + 1))
else
    echo "  PASS: hot reload: auth removed"
    PASS=$((PASS + 1))
fi
echo ""

# ── 9. Graceful shutdown (drain active connections) ──
echo "▶ Test: graceful shutdown"

# Restart daemon with a clean config for this test
"$DEVLB" stop >/dev/null 2>&1 || true
sleep 1

cat > "$TMPDIR_E2E/.devlb/devlb.yaml" <<EOF
services:
  - name: api
    port: $LISTEN_PORT
EOF
"$DEVLB" start >/dev/null 2>&1
sleep 1

# Verify daemon is ready
output=$("$DEVLB" status 2>&1 || true)
if ! echo "$output" | grep -q ":$LISTEN_PORT"; then
    echo "  FAIL: graceful shutdown: daemon did not start"
    FAIL=$((FAIL + 1))
fi

# Start a slow backend (takes 2s to respond)
python3 -c "
import http.server, time, sys
class SlowHandler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        time.sleep(2)
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b'drain-ok')
    def log_message(self, *args):
        pass
http.server.HTTPServer(('127.0.0.1', 18940), SlowHandler).serve_forever()
" &
SLOW_PID=$!
PIDS_TO_KILL+=($SLOW_PID)
sleep 0.5

"$DEVLB" route $LISTEN_PORT 18940 --label slow-backend >/dev/null 2>&1
sleep 0.3

# Start a slow request in the background (will take ~2s)
GRACEFUL_RESULT="$TMPDIR_E2E/graceful-result"
curl -s --max-time 10 "http://127.0.0.1:$LISTEN_PORT/" > "$GRACEFUL_RESULT" &
CURL_PID=$!
sleep 0.5  # let connection establish through the proxy

# Stop daemon while request is in-flight
"$DEVLB" stop >/dev/null 2>&1 &
STOP_PID=$!

# Wait for curl to complete — should succeed if drain works
wait "$CURL_PID" 2>/dev/null
curl_exit=$?

if [ "$curl_exit" -eq 0 ] && [ "$(cat "$GRACEFUL_RESULT")" = "drain-ok" ]; then
    echo "  PASS: graceful drain: in-flight request completed"
    PASS=$((PASS + 1))
else
    echo "  FAIL: graceful drain: in-flight request failed (exit=$curl_exit, body='$(cat "$GRACEFUL_RESULT" 2>/dev/null)')"
    FAIL=$((FAIL + 1))
fi

# Wait for stop to finish
wait "$STOP_PID" 2>/dev/null || true

# Verify daemon is stopped
output=$("$DEVLB" status 2>&1 || true)
assert_contains "daemon stopped after drain" "not running" "$output"

kill "$SLOW_PID" 2>/dev/null; wait "$SLOW_PID" 2>/dev/null || true
unset 'PIDS_TO_KILL[-1]'
echo ""

# ── Summary ──
echo "════════════════════════"
echo "  PASS: $PASS"
echo "  FAIL: $FAIL"
echo "════════════════════════"
[ "$FAIL" -eq 0 ] && exit 0 || exit 1
