# Changelog

All notable changes to Tunnerse are documented in this file.

## [1.1.0] - 2026-05-20

### Added

- Added CLI heartbeat requests for active foreground HTTP tunnels.
- Added daemon-side heartbeat monitoring for registered HTTP tunnel jobs.
- Added `GET /health/:tunnel_id` on the local daemon API to receive tunnel keep-alive signals.

### Fixed

- Fixed foreground HTTP tunnels staying active when the CLI process was interrupted or closed before the normal stop request completed.
- Improved tunnel cleanup after `Ctrl+C` by allowing the daemon to close the tunnel when CLI heartbeats stop.
- Fixed the Windows application icon by rebuilding `assets/icons/win/tunnerse.ico` with embedded `16x16`, `32x32`, `48x48`, `64x64`, `128x128`, and `256x256` variants.

### Changed

- The CLI now sends heartbeat requests every 3 seconds while a foreground HTTP tunnel is running.
- The daemon now closes the tunnel if no heartbeat is received for 10 seconds.
- Users should update the CLI and daemon together because the heartbeat depends on both components.

## [1.0.0] - 2026-05-18

### Added

- Initial public Tunnerse release.
- Added `tunnerse`, the developer CLI for opening and stopping foreground HTTP tunnels.
- Added `tunnerse-daemon`, the local daemon responsible for tunnel registration, polling, forwarding, shutdown, and per-tunnel logs.
- Added `tunnerse-server`, the public tunnel server responsible for tunnel registration, request queues, daemon responses, and HTTPS domain exposure.
- Added foreground HTTP tunnel creation from a local port.
- Added stateless in-memory tunnel lifecycle with no database or persistent counters.
- Added architecture-specific Windows installers for `amd64` and `x86`.
- Added Debian packages for the local CLI+daemon bundle and the public server.
- Added centralized project versioning through `internal/version/VERSION`.
- Added build scripts that inject the same version into binaries and artifact names.
- Added documentation assets, installer images, and icons under `assets/`.

### Supported

- Server-daemon communication over JSON.
- Base64-encoded request and response bodies inside JSON.
- Public request bodies up to 32 MiB.
- Local response bodies up to 32 MiB.
- Server response JSON up to 64 MiB.
- Per-tunnel server queue defaults to 256 requests.
- Per-tunnel pending public requests default to 256.
- Daemon forwarding concurrency limited to 128 requests per tunnel.
- Hop-by-hop header stripping while forwarding.
- Forced `Accept-Encoding: identity` when the daemon calls the local app.

### Known Limitations

- Tunnels are not persisted across daemon or server restarts.
- The protocol supports HTTP request/response traffic only.
- WebSockets, SSE, raw TCP, and large streaming transfers are not supported.
- Base64 is reliable for the polling protocol, but not the most efficient option for very large payloads.
- HTML/path rewriting is best-effort and only applies to uncompressed responses.
