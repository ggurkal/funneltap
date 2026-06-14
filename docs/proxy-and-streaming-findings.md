# funneltap proxy and streaming findings

Notes from an investigation into raw body handling, HMAC/webhook verification, SSE/WebSocket support, and what it would take for funneltap to match Tailscale Funnel's HTTP reverse-proxy behavior.

## Raw request body and HMAC verification

### What funneltap does today

For intercepted requests, funneltap:

1. Reads the full body into a `[]byte` with `io.ReadAll` (no JSON decode, gzip inflate, or charset conversion).
2. Stores an exact copy in memory (`append([]byte(nil), body...)`).
3. Exposes it via `GET /requests/{id}` as `bodyBase64` (base64 is only for JSON transport).
4. Forwards the same byte slice upstream via `bytes.NewReader(body)`, with headers cloned.

Relevant code: `internal/intercept/handler.go`, `internal/store/store.go`, `internal/api/api.go`.

### For security / HMAC workflows

**Body content is bit-identical to what was received** for typical webhook schemes (Stripe, GitHub, Svix, etc.) that sign the raw body (sometimes with a timestamp prefix).

To verify from a captured request:

```bash
curl -s http://localhost:9000/requests/42 | jq -r '.bodyBase64' | base64 -d > body.bin
# Use body.bin + signature header from the same JSON response
```

### Caveats

| Issue | Impact |
|-------|--------|
| Path rewriting | Upstream sees the route suffix (e.g. `/github`), not the public funnel path. Fine for body-only signatures; breaks schemes that sign URL/method (e.g. AWS SigV4). |
| Chunked → Content-Length | Wire framing can change on the outbound hop; payload bytes do not. |
| Hop-by-hop headers | Go's `http.Client` may drop/normalize `Connection`, `Transfer-Encoding`, etc. Signature headers are usually unaffected. |
| Size limit | Bodies over `MAX_BODY_BYTES` (default 10 MiB) are rejected before store/forward. |
| Full buffering | Body must fit in memory; not streamed through. |
| Tailscale Funnel | Another hop outside funneltap; should be transparent for bodies, but is not under funneltap's control. |

### Upstream verification

If the app behind funneltap verifies signatures on receive, body-based webhooks should still work when the app only cares about body + signature headers. Verification that depends on the original request URL/path may fail because funneltap rewrites the path when proxying.

---

## SSE and WebSocket support

### funneltap: not supported

funneltap is built for short request/response HTTP (webhooks). It does not support SSE or WebSockets today.

**WebSockets**

- No `Upgrade` / `Connection` handling and no `Hijack` for bidirectional TCP relay.
- Uses `http.Client.Do`, copies response headers, then `io.Copy` on the body.
- A `101 Switching Protocols` response does not become a working WebSocket tunnel.

**SSE**

- In theory still HTTP, but blocked in practice by:
  - `PROXY_TIMEOUT` (default 30s) on the entire upstream round trip via `http.Client{Timeout}`.
  - No explicit flush handling for `text/event-stream`.
  - Full request body buffering before proxying.

### Tailscale Funnel: mostly yes, not first-class

Funnel is an HTTPS reverse proxy to a local HTTP service. Tailscale terminates TLS on the node and forwards decrypted HTTP to `127.0.0.1`.

Official docs do not explicitly list WebSocket or SSE as supported features. Documented limits include ports (`443`, `8443`, `10000`), TLS-only, and bandwidth caps.

**WebSockets**

- Used in practice through `tailscale serve` / `funnel`.
- Known issues:
  - Query parameters on WebSocket upgrade URLs may be dropped through Funnel (tailscale#18651).
  - Connections can drop every 10-40s through Serve on some setups (tailscale#18827).

**SSE**

- Should work in principle (long-lived HTTP response stream).
- No explicit "SSE not supported" limitation in docs; behavior is the usual long-connection reverse-proxy semantics.

### funneltap vs Funnel

| Layer | WebSockets | SSE |
|-------|------------|-----|
| Tailscale Funnel (direct to app) | Often works; known edge cases | Likely works |
| funneltap in the middle | No | No in practice |

funneltap configures Funnel to hit its intercept server, not the upstream directly:

```text
Internet → Tailscale Funnel → funneltap intercept → your app
```

For SSE/WebSockets, bypass funneltap:

```text
Internet → Tailscale Funnel → your app
```

---

## What funneltap needs to match Tailscale Funnel (HTTP proxy)

Goal: match **Tailscale Funnel's HTTP reverse proxy** on the localhost hop. Not required: TLS termination, TCP forwarders, file serving, PROXY protocol, public DNS/certs (Tailscale handles those before funneltap).

### Current gaps

| Capability | Tailscale Funnel | funneltap today |
|------------|------------------|-----------------|
| Stream request body | Yes | No — `io.ReadAll` buffers first |
| Stream response body | Yes | Partial — `io.Copy`, but upstream may already be timed out |
| Long-lived connections | Yes | No — `http.Client{Timeout: 30s}` |
| WebSocket upgrade | Yes (with quirks) | No |
| SSE / chunked streaming | Yes | No in practice |
| Hop-by-hop headers | Handled by proxy | Cloned blindly |
| Host rewriting | Yes | Clones client Host (intercept server) |

Root cause: `internal/intercept/handler.go` buffers everything, uses a single-shot `Client.Do`, then copies the response.

### Recommended changes

#### 1. Replace proxy with `httputil.ReverseProxy`

Use Go's reverse proxy (WebSocket upgrades supported since Go 1.12) instead of a hand-rolled `http.Client` loop:

```go
proxy := &httputil.ReverseProxy{
    Rewrite: func(pr *httputil.ProxyRequest) {
        pr.SetURL(upstreamURL)
        pr.Out.URL.Path = upstreamPath
        pr.Out.URL.RawQuery = pr.In.URL.RawQuery
    },
    Transport: streamingTransport,
}
```

#### 2. Split timeouts

- Keep dial / TLS handshake timeout (e.g. 30s).
- Optional response-header timeout for slow backends.
- No timeout on the full request for streaming connections.

`PROXY_TIMEOUT` should not cover the entire round trip once bodies stream.

#### 3. Dual-mode: capture vs tunnel

| Request type | Behavior |
|--------------|----------|
| Normal HTTP (POST webhooks) | Tee body → store → forward (preserve HMAC-friendly capture) |
| `Upgrade: websocket` | Transparent tunnel; metadata-only capture |
| `Accept: text/event-stream` or long GET | Stream through; optional header-only capture |

#### 4. Capture without breaking streaming

For the capture path, use `io.TeeReader` with a size-limited buffer instead of `io.ReadAll` before forwarding. For WebSocket/SSE, do not capture bodies (or only log connection metadata).

#### 5. WebSocket specifics

- Preserve query string on upgrade (`RawQuery`).
- Set `Host` to upstream host.
- Keep connection open until client or upstream closes.
- UI: show streaming/open state, not a completed 30s request.

#### 6. SSE specifics

- Rely on `ReverseProxy` streaming.
- Do not block `Flusher` / `ResponseController`.
- Mark proxy status as active until the stream ends.

#### 7. Store and UI model

Extend `ProxyInfo` for long-lived connections:

```go
type ProxyInfo struct {
    Status     int
    DurationMs int64
    Error      string
    Streaming  bool
    ClosedAt   *time.Time
}
```

Update proxy info on connection close, not when response headers arrive.

#### 8. Optional per-route bypass

Route flag e.g. `inspect: false`:

```text
/hooks → funneltap intercept (capture)
/ws    → upstream direct via funnel (no inspect)
```

Lowest-effort escape hatch for WebSocket routes.

### Suggested implementation order

1. `ReverseProxy` + timeout split — fixes SSE and long polling quickly.
2. WebSocket tunnel path via `ReverseProxy` upgrade handling.
3. Tee-based capture for normal HTTP (preserve webhook/HMAC inspection).
4. Store/UI updates for streaming connections.
5. Per-route `inspect: false` bypass (optional).

### Bottom line

To be on par with Tailscale Funnel for dynamic HTTP apps: **stop buffering by default**, **remove the global client timeout for streams**, and **use `httputil.ReverseProxy` with a capture-vs-tunnel split**. Keep full inspection for webhooks; use transparent tunnel + lightweight metadata for streaming traffic.
