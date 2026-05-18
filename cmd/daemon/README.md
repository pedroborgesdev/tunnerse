# Tunnerse Daemon

`cmd/daemon` is the local Tunnerse service. It listens on `http://localhost:9988` by default and bridges the CLI, the public Tunnerse server, and the application running on the developer machine.

The daemon is the process that actually forwards public tunnel requests to `localhost:<port>`.

## Responsibilities

- Expose a local HTTP API for the CLI.
- Register requested tunnels with the public server.
- Keep active tunnel loops in memory.
- Poll the public server for queued requests.
- Forward requests to a local HTTP service.
- Send local responses back to the public server.
- Write per-tunnel logs that the CLI can stream.
- Run as a Windows service when launched by the Windows service manager.

## State Model

The daemon does not use a database. It keeps active tunnel URLs and running loop jobs in process memory only.

Persistent files are limited to logs:

```text
~/.tunnerse/
  logs/
```

The base data directory can be overridden with:

```env
TUNNERSE_DATA_DIR=/custom/path
```

Each tunnel gets its own log file under the configured `logs/` directory.

## Configuration

The daemon local API currently listens on `http://localhost:9988`.

The only daemon runtime environment variable is:

```env
TUNNERSE_DATA_DIR=/custom/path
```

It changes where logs are stored. When unset, the daemon uses `~/.tunnerse/logs`.

The public server URL is sent by the CLI in the `/http` request. The bundled CLI currently sends `https://tunnerse.com`.

## Local API

```text
GET  /health
POST /http
GET  /http/logs/:tunnel_id
POST /http/stop
```

Health check:

```bash
curl http://localhost:9988/health
```

### Create A Tunnel

```http
POST /http
Content-Type: application/json
```

```json
{
  "name": "my-app",
  "port": "3000",
  "server_url": "https://tunnerse.com"
}
```

The daemon validates `server_url`, posts `{ "name": "my-app" }` to the public server `/register`, extracts the generated tunnel ID, stores the final tunnel URL in memory, creates a loop job, and returns:

```json
{
  "code": "success",
  "message": "Operation successful",
  "data": {
    "message": "HTTP tunnel has been registered",
    "subdomain": false,
    "tunnel": "my-app-a1b2c3d4"
  },
  "status": 200
}
```

### Tail Logs

```http
GET /http/logs/:tunnel_id?offset=0
```

Response:

```json
{
  "code": "success",
  "message": "Operation successful",
  "data": {
    "tunnel_id": "my-app-a1b2c3d4",
    "logs": "...",
    "offset": 1234
  },
  "status": 200
}
```

The CLI keeps the last `offset` and asks only for new content.

### Stop A Tunnel

```http
POST /http/stop
Content-Type: application/json
```

```json
{
  "tunnel_id": "my-app-a1b2c3d4"
}
```

The daemon stops the local loop job, removes the tunnel from memory, and asynchronously posts to the public server `/close`.

## Tunnel Loop

Each active tunnel creates one `LoopJob`.

The loop:

1. Opens a per-tunnel log file.
2. Starts a local healthcheck worker.
3. Starts a server ping worker.
4. Polls the public server with `GET <tunnel-url>/tunnel`.
5. Forwards fetched requests to the local app.
6. Posts responses to `POST <tunnel-url>/response`.
7. Stops when the CLI requests shutdown, the public server closes the tunnel, the local app repeatedly fails healthchecks, or repeated polling errors trip the loop guard.

Concurrency and limits:

- Maximum concurrent forwarded requests per tunnel: `128`.
- Maximum request body read from the server: `32 MiB`.
- Maximum local response body: `32 MiB`.
- Polling client timeout to the public server: `75s`.
- Local forwarding HTTP client timeout: `30s`.

## Forwarding Behavior

The public server sends requests to the daemon as JSON. The body is base64 encoded. The daemon decodes it and forwards it to the local app.

The daemon removes hop-by-hop headers before forwarding and sets:

```http
Accept-Encoding: identity
```

When the local response is not compressed, the daemon can adjust HTML/assets for the active routing mode:

- Path routing: rewrites absolute paths so they remain under `/<tunnel-id>/`.
- Subdomain routing: injects Tunnerse tunnel metadata into supported content.

The daemon strips hop-by-hop response headers, base64 encodes the response body, and posts the response back to the server with the original `Tunnerse-Request-Token`.

## Healthchecks

Local app healthcheck:

- Starts after a 5 second delay.
- Calls the local app root every 60 seconds.
- Closes the tunnel after 10 consecutive failures.

Server challenge ping:

- Starts after a 5 second delay.
- Sends `HEAD <tunnel-url>/_tunnerse_healthcheck` every 10 seconds.
- Expects the server/daemon round trip to return `Tunnerse: healthcheck-conclued`.

## Build

```bash
go build -o tunnerse-daemon ./cmd/daemon
```

On Windows:

```powershell
go build -o tunnerse-daemon.exe ./cmd/daemon
```

## Run Locally

```bash
go run ./cmd/daemon
```

The daemon logs startup information, including the resolved data directory and logs directory.

## Windows Service

On Windows, the daemon can run under the service name:

```text
TunnerseDaemon
```

When executed by the service manager it handles stop and shutdown events gracefully. On non-Windows systems the same binary runs as a foreground process and can be managed by systemd or another supervisor.

## Troubleshooting

`HTTP tunnel not found` means the tunnel ID is not present in the daemon memory map, usually because the daemon restarted or the tunnel already stopped.

`target server did not respond` means repeated polling/forwarding errors closed the local loop.

`local API failed 10 times` means the configured local port stopped responding to healthchecks.

`local response body too large` means the local app returned more than 32 MiB.

## Related Docs

- [CLI command](../cli/README.md)
- [Public server](../server/README.md)
- [Project overview](../../README.md)
