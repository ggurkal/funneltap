#!/usr/bin/env bash
# Manual integration test for proxy + streaming (see docs/plan-proxy-streaming.md).
#
# Prerequisites:
#   1. funneltap running with a route, e.g. POST /routes {"path":"/one","target":"127.0.0.1:9876"}
#   2. Go installed (for test backend)
#
# Env:
#   API          funneltap API (default http://127.0.0.1:9000)
#   PORT         funneltap intercept PORT (required)
#   BACKEND_PORT test backend port (default 9876)
#   ROUTE_MOUNT  funnel mount path (default /one)
set -euo pipefail

API="${API:-http://127.0.0.1:9000}"
PORT="${PORT:?set PORT to funneltap intercept port}"
BACKEND_PORT="${BACKEND_PORT:-9876}"
ROUTE_MOUNT="${ROUTE_MOUNT:-/one}"
INTERCEPT="http://127.0.0.1:${PORT}"
BACKEND_DIR="$(cd "$(dirname "$0")/test-streaming-backend" && pwd)"

backend_pid() {
  ss -ltnp 2>/dev/null | grep "127.0.0.1:${BACKEND_PORT}" | sed -n 's/.*pid=\([0-9]*\).*/\1/p' | head -1
}

start_backend() {
  local pid
  pid=$(backend_pid || true)
  if [[ -n "${pid}" ]]; then
    kill "${pid}" 2>/dev/null || true
    sleep 0.2
  fi
  (cd "${BACKEND_DIR}" && go run . -addr "127.0.0.1:${BACKEND_PORT}") &
  sleep 1
  echo "backend started on :${BACKEND_PORT}"
}

stop_backend() {
  local pid
  pid=$(backend_pid || true)
  if [[ -n "${pid}" ]]; then
    kill "${pid}" 2>/dev/null || true
  fi
}

internal_path() {
  local suffix="$1"
  if [[ "${ROUTE_MOUNT}" == "/" ]]; then
    echo "/.ft${suffix}"
  else
    echo "/.ft${ROUTE_MOUNT}${suffix}"
  fi
}

trap stop_backend EXIT
start_backend

POST_URL="${INTERCEPT}$(internal_path "/test")"
SSE_URL="${INTERCEPT}$(internal_path "/sse")?count=3"
WS_URL="ws://127.0.0.1:${PORT}$(internal_path "/ws")"

echo "== POST /test (inspect) =="
curl -sS -o /tmp/ft-post-out.txt -w "HTTP %{http_code}\n" \
  -X POST "${POST_URL}" \
  -H "Content-Type: application/json" \
  -d '{"hello":"world"}'
cat /tmp/ft-post-out.txt
echo

echo "== GET /sse (tunnel, 3 events) =="
curl -sS -N --max-time 10 "${SSE_URL}" | head -5
echo

echo "== GET /ws (tunnel) =="
(cd "${BACKEND_DIR}" && go run . -client "${WS_URL}")
echo

echo "== API verification =="
python3 - <<PY
import base64
import json
import sys
import urllib.request

api = "${API}".rstrip("/")

with urllib.request.urlopen(f"{api}/requests") as resp:
    rows = json.load(resp)

if not rows:
    print("FAIL: no captured requests", file=sys.stderr)
    sys.exit(1)

by_path = {}
for r in rows:
    path = r.get("path", "")
    by_path[path.split("?")[0]] = r

def get_detail(rid):
    with urllib.request.urlopen(f"{api}/requests/{rid}") as resp:
        return json.load(resp)

errors = []

post = by_path.get("/test")
if not post:
    errors.append("missing captured POST /test")
else:
    d = get_detail(post["id"])
    body = d.get("bodyBase64", "")
    proxy = d.get("proxy", {})
    if proxy.get("streaming"):
        errors.append("POST /test should not be streaming")
    if not body:
        errors.append("POST /test should have bodyBase64")
    else:
        raw = base64.b64decode(body).decode()
        if "hello" not in raw:
            errors.append(f"POST /test body missing payload: {raw!r}")
    if not proxy.get("status"):
        errors.append("POST /test should have final proxy status")
    print(f"OK POST /test id={post['id']} status={proxy.get('status')} body captured")

sse = by_path.get("/sse")
if not sse:
    errors.append("missing captured GET /sse")
else:
    d = get_detail(sse["id"])
    proxy = d.get("proxy", {})
    if d.get("bodyBase64"):
        errors.append("GET /sse should not capture body")
    print(f"OK GET /sse id={sse['id']} streaming={proxy.get('streaming')} status={proxy.get('status')}")

ws = by_path.get("/ws")
if not ws:
    errors.append("missing captured GET /ws")
else:
    d = get_detail(ws["id"])
    proxy = d.get("proxy", {})
    if d.get("bodyBase64"):
        errors.append("GET /ws should not capture body")
    print(f"OK GET /ws id={ws['id']} streaming={proxy.get('streaming')} status={proxy.get('status')}")

if errors:
    for e in errors:
        print(f"FAIL: {e}", file=sys.stderr)
    sys.exit(1)

print("\nAll API checks passed.")
PY

echo ""
echo "Latest requests:"
curl -sS "${API}/requests" | python3 -c "
import json, sys
for r in json.load(sys.stdin)[:5]:
    p = r['proxy']
    print(f\"{r['id']:>3} {r['method']:<4} {r['path']:<20} status={p.get('status')} streaming={p.get('streaming')} {p.get('durationMs')}ms\")
"
