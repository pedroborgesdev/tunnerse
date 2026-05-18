# Tunnerse Server

`cmd/server` is the public Tunnerse server. It registers tunnel names, receives public HTTP traffic, queues requests for connected daemons, receives daemon responses, writes responses back to public clients, and exposes configured domains over HTTPS.

This command is meant to run on public infrastructure behind `tunnerse.com` or another domain you control.

![Tunnerse running page](../../assets/readme/website-tunnerse-running.png)

## Responsibilities

- Register tunnel names through `/register`.
- Keep active tunnels in memory.
- Queue public requests for each tunnel.
- Match daemon responses to public requests by `Tunnerse-Request-Token`.
- Close tunnels when requested, inactive, or expired.
- Serve embedded HTML pages for running, not found, timeout, and local API error states.
- Start an HTTP-to-HTTPS redirect listener on `:80`.
- Start an HTTPS reverse proxy listener on `:443` from `tunnerse.config`.

## State Model

The server does not use a database. Registered tunnels live in an in-memory map inside the process.

Each tunnel owns:

- A bounded request queue.
- A map of pending request tokens to response channels.
- An inactivity timer.
- An optional max lifetime timer.
- A close signal used to release pending requests safely.

If the server process restarts, all active tunnels are lost and clients must register new tunnels.

## Routing Modes

Path routing, default:

```text
https://tunnerse.com/my-app-a1b2c3d4/path
```

Subdomain routing:

```text
https://my-app-a1b2c3d4.tunnerse.com/path
```

Use:

```env
SUBDOMAIN=false
```

or:

```env
SUBDOMAIN=true
```

In subdomain mode, the tunnel name is extracted from the first host segment. In path mode, the tunnel name is extracted from `/:name/...`.

## Configuration

The server loads `.env` when present. Missing values fall back to defaults.

```env
HTTPPort=8080
SUBDOMAIN=false
WARNS_ON_HTML=true
TUNNEL_LIFE_TIME=86400
TUNNEL_INACTIVITY_LIFE_TIME=86400
TUNNEL_REQUEST_TIMEOUT=30
TUNNEL_QUEUE_SIZE=256
TUNNEL_MAX_PENDING_REQUESTS=256
```

Options:

- `HTTPPort`: local Gin API port behind the expose proxy. Default: `8080`.
- `SUBDOMAIN`: enables subdomain tunnel routing when `true`. Default: `false`.
- `WARNS_ON_HTML`: serves embedded HTML status pages for user-facing tunnel errors. Default: `true`.
- `TUNNEL_LIFE_TIME`: maximum lifetime for a registered tunnel, in seconds. Set `0` to disable the max lifetime timer.
- `TUNNEL_INACTIVITY_LIFE_TIME`: closes a tunnel after this many seconds without tunnel activity.
- `TUNNEL_REQUEST_TIMEOUT`: timeout while waiting for a daemon response to a public request.
- `TUNNEL_QUEUE_SIZE`: buffered request queue size per tunnel.
- `TUNNEL_MAX_PENDING_REQUESTS`: maximum in-flight public requests waiting for daemon responses.

## Domain And TLS Expose

Domain routing is configured in `tunnerse.config`.

```ini
[domains]
*.your-server-domain.com=8080
your-server-domain.com=8080

[tls]
cert_file=certs/certificates/your-server-domain.com.crt
key_file=certs/certificates/your-server-domain.com.key
```

Rules:

- `[domains]` is required and must contain at least one domain.
- Wildcard domains are supported with `*.example.com`.
- `[tls]` is required.
- `cert_file` and `key_file` are required.
- TLS paths can be absolute or relative to the `tunnerse.config` file.

At startup, the server:

1. Loads `.env`.
2. Loads `tunnerse.config`.
3. Starts `:80` and redirects all HTTP traffic to HTTPS.
4. Starts `:443` and reverse proxies configured hosts to `http://localhost:<port>`.
5. Starts the Gin API on `HTTPPort`.

## Build

```bash
go build -o tunnerse-server ./cmd/server
```

On Windows:

```powershell
go build -o tunnerse-server.exe ./cmd/server
```

## Run Locally

```bash
go run ./cmd/server
```

Health check:

```bash
curl http://localhost:8080/health
```

Local runs may need elevated privileges because the expose layer binds ports `80` and `443`.

## Public API

Utility endpoints:

```text
GET  /health
GET  /favicon.ico
HEAD /favicon.ico
GET  /favicon.ico/
HEAD /favicon.ico/
```

Subdomain mode:

```text
POST /register
GET  /tunnel
POST /response
POST /close
GET  /
HEAD /_tunnerse_healthcheck
*    /*
```

Path mode:

```text
POST /register
GET  /:name/tunnel
POST /:name/response
POST /:name/close
GET  /:name/
HEAD /:name/_tunnerse_healthcheck
*    /:name/*
```

## Register Tunnel

```http
POST /register
Content-Type: application/json
```

```json
{
  "name": "my-app"
}
```

Tunnel names must match:

```text
^[a-z0-9-]{1,20}$
```

The server appends an 8-character random suffix to avoid collisions:

```json
{
  "code": "success",
  "message": "Operation successful",
  "data": {
    "message": "tunnel has been registered",
    "subdomain": false,
    "tunnel": "my-app-a1b2c3d4"
  },
  "status": 200
}
```

## Daemon Polling

The daemon calls `/tunnel` to fetch one queued public request.

The server returns JSON:

```json
{
  "method": "POST",
  "path": "/webhook",
  "headers": {
    "Content-Type": ["application/json"],
    "Tunnerse-Request-Token": ["..."]
  },
  "body": "eyJvayI6dHJ1ZX0=",
  "host": "tunnerse.com",
  "request_id": "",
  "token": "..."
}
```

The `body` field is base64 encoded. The daemon decodes it before forwarding to the local app.

## Daemon Response

The daemon sends the local app response to `/response`.

```json
{
  "status_code": 200,
  "headers": {
    "Content-Type": ["application/json"]
  },
  "body": "eyJzdGF0dXMiOiJvayJ9",
  "token": "..."
}
```

The server decodes `body`, applies headers except hop-by-hop headers, writes `status_code`, and sends the body to the original public client.

## Close Tunnel

```http
POST /close
Content-Type: application/json
```

```json
{
  "name": "my-app-a1b2c3d4"
}
```

The server removes the tunnel from memory, closes pending response channels, and releases waiters.

## Public Request Handling

When a public client calls a tunnel URL:

1. The server validates that the tunnel exists.
2. It creates a UUID request token.
3. It reads the request body up to 32 MiB.
4. It stores a response channel in `pendingRequests[token]`.
5. It queues the cloned request for the daemon.
6. It waits up to `TUNNEL_REQUEST_TIMEOUT`.
7. It writes the daemon response to the public client.

If the queue is full, pending request limit is reached, the daemon does not answer in time, or the client disconnects, the server returns an error. With `WARNS_ON_HTML=true`, user-facing tunnel failures use embedded HTML pages.

## Limits

- Maximum public request body read by the server: 32 MiB.
- Maximum daemon response JSON read by the server: 64 MiB.
- Maximum decoded daemon response body: 32 MiB.
- Default request queue size per tunnel: `256`.
- Default maximum pending requests per tunnel: `256`.
- Default public request wait timeout: `30s`.
- API server read timeout: `90s`.
- API server write timeout: `max(TUNNEL_REQUEST_TIMEOUT + 30s, 90s)`.

## Embedded Assets

The server embeds files from `internal/server/embed`:

- `running.html`
- `notfound.html`
- `timeout.html`
- `localerror.html`
- `icon.png`
- `icon.webp`
- `favicon.ico`

Embedded pages also set a `Tunnerse` response header, such as `tunnel-working`, `tunnel-not-found`, `tunnel-timeout`, or `local-api-error`.

## Troubleshooting

`error to load config` means `tunnerse.config` is missing or invalid.

`domain not configured` means the request host did not match any `[domains]` entry.

`tunnel not found` means the tunnel is not present in the in-memory map, usually because it expired, was closed, or the server restarted.

`timeout` means a public request waited longer than `TUNNEL_REQUEST_TIMEOUT` for the daemon response.

`local-api-error` means the daemon could not reach the local app and returned a 503 tunnel response.

## Related Docs

- [CLI command](../cli/README.md)
- [Local daemon](../daemon/README.md)
- [Project overview](../../README.md)
