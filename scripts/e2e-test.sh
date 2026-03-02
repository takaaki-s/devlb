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
    "$DEVLB" stop 2>/dev/null || true
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

# ── 6. Stop ──
echo "▶ Test: stop"
"$DEVLB" stop >/dev/null 2>&1
output=$("$DEVLB" status 2>&1 || true)
assert_contains "daemon stopped" "not running" "$output"
echo ""

# ── Summary ──
echo "════════════════════════"
echo "  PASS: $PASS"
echo "  FAIL: $FAIL"
echo "════════════════════════"
[ "$FAIL" -eq 0 ] && exit 0 || exit 1
