#!/usr/bin/env bash
# Exercise all funneltap routes with a mix of successful and failed requests.
#
# Prerequisites:
#   1. funneltap running with routes configured in the UI (or via POST /routes).
#   2. Route targets should point at BACKEND_PORT (default 9876) for success cases.
#
# Env:
#   API          funneltap API base URL (default http://127.0.0.1:9000)
#   INTERCEPT    intercept server base URL (default http://127.0.0.1:$PORT if PORT is set)
#   PORT         same as funneltap PORT; needed for direct intercept curls (optional if using funnel only)
#   FUNNEL       public funnel base fallback (optional; routes may already have publicURL)
#   BACKEND_PORT port for the temporary python backend (default 9876)
set -euo pipefail

API="${API:-http://127.0.0.1:9000}"
PORT="${PORT:-}"
if [[ -n "${PORT}" ]]; then
  INTERCEPT="${INTERCEPT:-http://127.0.0.1:${PORT}}"
else
  INTERCEPT="${INTERCEPT:-}"
fi
FUNNEL="${FUNNEL:-}"
BACKEND_PORT="${BACKEND_PORT:-9876}"

backend_pid() {
  ss -ltnp 2>/dev/null | grep "127.0.0.1:${BACKEND_PORT}" | sed -n 's/.*pid=\([0-9]*\).*/\1/p' | head -1
}

start_backend() {
  if backend_pid >/dev/null; then
    return
  fi
  python3 - <<PY &
from http.server import HTTPServer, BaseHTTPRequestHandler

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.end_headers()
        self.wfile.write(f"backend-get {self.path}".encode())

    def do_POST(self):
        n = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(n)
        self.send_response(201)
        self.end_headers()
        self.wfile.write(b"backend-post:" + body)

    def do_PUT(self):
        n = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(n)
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b"backend-put:" + body)

    def log_message(self, *args):
        pass

HTTPServer(("127.0.0.1", ${BACKEND_PORT}), Handler).serve_forever()
PY
  sleep 0.3
}

stop_backend() {
  local pid
  pid=$(backend_pid || true)
  if [[ -n "${pid}" ]]; then
    kill "${pid}" 2>/dev/null || true
    sleep 0.2
  fi
}

send_requests() {
  local phase="$1"
  API="$API" INTERCEPT="$INTERCEPT" FUNNEL="$FUNNEL" PHASE="$phase" python3 - <<'PY'
import json
import os
import subprocess
import sys
import urllib.request

api = os.environ["API"].rstrip("/")
intercept = os.environ.get("INTERCEPT", "").rstrip("/")
funnel_base = os.environ.get("FUNNEL", "").rstrip("/")
phase = os.environ["PHASE"]

with urllib.request.urlopen(f"{api}/routes") as resp:
    routes = json.load(resp)

if not routes:
    print("No routes configured. Add routes in the UI or POST /routes first.", file=sys.stderr)
    sys.exit(1)

print(f"Found {len(routes)} route(s)")

# Per-route suffixes and methods (cycled by index).
cases = [
    ("GET", "/probe", None, []),
    ("POST", "/event", '{"source":"send-test-requests"}', ["-H", "Content-Type: application/json"]),
    ("PUT", "/deploy", "v1.0.0", []),
    ("POST", "/hooks/github", '{"ref":"refs/heads/main"}', ["-H", "Content-Type: application/json", "-H", "X-GitHub-Event: push"]),
]


def internal_prefix(mount: str) -> str:
    return "/.ft" if mount == "/" else "/.ft" + mount


def funnel_url(route: dict, suffix: str) -> str | None:
    public = (route.get("publicURL") or "").rstrip("/")
    if public:
        return public + suffix
    mount = route["path"].rstrip("/") or ""
    if funnel_base:
        return funnel_base + mount + suffix
    return None


def curl_request(label: str, method: str, url: str, body: str | None, extra_args: list[str]) -> None:
    cmd = ["curl", "-sS", "-o", "/dev/null", "-w", f"{label} -> %{{http_code}}\n", "-X", method, url, *extra_args]
    if body is not None:
        cmd.extend(["-d", body])
    subprocess.run(cmd, check=False)


if phase == "success":
    print("\n== successful requests (backend up) ==")
elif phase == "failure":
    print("\n== failed requests (backend down) ==")
else:
    print(f"unknown phase: {phase}", file=sys.stderr)
    sys.exit(1)

for i, route in enumerate(routes):
    mount = route["path"]
    method, suffix, body, extra = cases[i % len(cases)]
    label_path = mount.rstrip("/") + suffix

    intercept_url = intercept + internal_prefix(mount) + suffix
    if intercept:
        curl_request(f"{method} intercept {label_path}", method, intercept_url, body, extra)
    else:
        print(f"  (skip intercept for {label_path}: set PORT or INTERCEPT)")

    furl = funnel_url(route, suffix)
    if furl:
        curl_request(f"{method} funnel {label_path}", method, furl, body, extra)
    elif funnel_base == "":
        print(f"  (skip funnel for {label_path}: set FUNNEL or configure publicURL on route)")

if phase == "failure":
    print("\n== unmatched (expect 404, not captured) ==")
    if intercept:
        curl_request("GET intercept /.ft/unmatched", "GET", intercept + "/.ft/unmatched", None, [])
    if funnel_base:
        curl_request("GET funnel /unmatched", "GET", funnel_base + "/unmatched?x=1", None, [])
PY
}

echo "== funneltap route smoke test =="
start_backend
send_requests success

stop_backend
send_requests failure

echo ""
echo "== restart backend =="
start_backend

echo ""
echo "== captured (latest 15) =="
curl -sS "${API}/requests" | python3 -c "
import json, sys
rows = json.load(sys.stdin)[:15]
if not rows:
    print('(no captured requests)')
for r in rows:
    p = r['proxy']
    route = r.get('routePath') or '-'
    err = ' ERR' if p.get('error') else ''
    print(f\"{r['id']:>3} {r['method']:<4} {r['path']:<30} {route:<12} {p['status']} {p['durationMs']}ms{err}\")
"
