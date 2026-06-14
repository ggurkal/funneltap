# funneltap

Inspect HTTP traffic forwarded through [Tailscale Funnel](https://tailscale.com/kb/1223/tailscale-funnel). funneltap sits between the public internet and your local services: it logs each request, proxies it to an upstream target, and shows everything in a built-in web UI.

## How it works

```text
Internet
  → tailscale funnel (--set-path /hooks)
  → intercept server (127.0.0.1:PORT)
  → route lookup
  → upstream target (e.g. http://localhost:3000)
```

You define **routes** in the UI. Each route maps a funnel mount path (for example `/hooks`) to a local upstream (for example `localhost:3000`). funneltap configures `tailscale funnel` to send traffic to its intercept server, not directly to your app — that way every request is captured before it is proxied upstream.

Unmatched paths return `404` and are not stored.

## Requirements

- Go 1.25+
- [Tailscale](https://tailscale.com/) with Funnel enabled on your tailnet

## Install

Download a binary for Linux or macOS from the [GitHub releases](https://github.com/ggurkal/funneltap/releases) page.

Or install with Go:

```bash
go install github.com/ggurkal/funneltap/cmd/funneltap@latest
```

Or build from source:

```bash
git clone https://github.com/ggurkal/funneltap.git
cd funneltap
go build -o funneltap ./cmd/funneltap
```

## Usage

Start funneltap:

```bash
funneltap
```

On startup it prints the intercept port and API address. Open the UI at `http://localhost:9000/ui/` (or whatever `API_PORT` is set to).

1. Click **Routes** and add a route — enter a mount path (e.g. `/hooks`) and an upstream target (e.g. `localhost:3000`).
2. funneltap runs `tailscale funnel` with `--set-path <path>` pointing at the intercept server under `/.ft<path>`, not directly at your upstream, and shows the public URL.
3. Send requests to the public URL. They appear in the request list with method, path, headers, body, and proxy status.

Use **Ctrl+C** for a clean shutdown: funnel mounts are torn down and the recovery file is removed.

### Target formats

Upstream targets accept flexible input:

| Input | Normalized |
|-------|------------|
| `8080` or `:8080` | `http://localhost:8080` |
| `localhost:3000` | `http://localhost:3000` |
| `192.168.0.1:8080` | `http://192.168.0.1:8080` |
| `https://api.example.com` | `https://api.example.com` |

## Configuration

Environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | random ephemeral port | Intercept server port (localhost only) |
| `API_PORT` | `9000` | API and UI port |
| `PROXY_TIMEOUT` | `30s` | Upstream proxy timeout |
| `MAX_REQUESTS` | `500` | Max captured requests in memory |
| `MAX_BODY_BYTES` | `10485760` (10 MiB) | Max request body size |
| `FUNNELTAP_ROUTES_FILE` | `/tmp/funneltap-routes.json` | Crash-recovery checkpoint file |

Example:

```bash
PORT=8081 API_PORT=9000 funneltap
```

## API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/requests` | List captured requests (`?after=<id>`, `?route=<id>`) |
| `GET` | `/requests/{id}` | Request detail |
| `DELETE` | `/requests` | Clear all requests |
| `GET` | `/routes` | List active routes |
| `POST` | `/routes` | Create route (`{"path":"/hooks","target":"localhost:3000"}`) |
| `DELETE` | `/routes/{id}` | Delete route |
| `GET` | `/recovery` | Check for crash-recovery data |
| `POST` | `/recovery/restore` | Restore routes from checkpoint |
| `POST` | `/recovery/dismiss` | Dismiss recovery prompt |

The UI is served at `/ui/`.

## Crash recovery

Route state is checkpointed to disk on every add or delete. If funneltap exits uncleanly (crash, `kill -9`), the next startup shows a recovery prompt in the UI. A graceful shutdown deletes the checkpoint file.

## Development

```bash
go test ./...
```

## License

MIT — see [LICENSE](LICENSE).
