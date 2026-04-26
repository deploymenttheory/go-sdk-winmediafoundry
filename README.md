# go-sdk-uupdump

[![Go Report Card](https://goreportcard.com/badge/github.com/deploymenttheory/go-sdk-uupdump)](https://goreportcard.com/report/github.com/deploymenttheory/go-sdk-uupdump)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/github/go-mod/go-version/deploymenttheory/go-sdk-uupdump)](https://github.com/deploymenttheory/go-sdk-uupdump)
![Status: Experimental](https://img.shields.io/badge/status-experimental-orange)

A native Go implementation of the Windows Unified Update (SOAP) protocol. Queries Microsoft's
`fe3.delivery.mp.microsoft.com` update endpoints directly — no intermediary service
required — to discover Windows builds, resolve CDN download URLs, and stream ESD/CAB
files from `tlu.dl.delivery.mp.microsoft.com`.

## What it does

| Capability | Detail |
|---|---|
| Build discovery | SyncUpdates SOAP call against live Windows Update endpoints |
| File URL resolution | GetExtendedUpdateInfo2 (EUI2) returns pre-signed Microsoft CDN URLs |
| Streaming download | Proxy any file directly from CDN via the REST API |
| Build catalog | SQLite-backed store with full-text search, pagination, and change feed |
| Change feed | Server-Sent Events stream of build discoveries and updates |
| Background watcher | Configurable poll interval keeps catalog current automatically |
| REST API | mTLS-protected HTTP server exposing all catalog and live-query operations |

## Architecture

```
Windows Update SOAP endpoints
  fe3.delivery.mp.microsoft.com   ← SyncUpdates (build discovery)
  fe3cr.delivery.mp.microsoft.com ← GetExtendedUpdateInfo2 (CDN file URLs)
          │
          ▼
  wuproto/soap   ← SOAP protocol implementation
          │
          ▼
  winupdate      ← Service layer (orchestrates SOAP + catalog)
          │
   ┌──────┴──────┐
   ▼             ▼
catalog/store  api/    ← SQLite catalog + mTLS REST API
(SQLite)       (HTTP)
```

CDN file URLs returned by EUI2 point to:
```
https://tlu.dl.delivery.mp.microsoft.com/filestreamingservice/files/<sha1>?P1=…&P2=…&P3=…&P4=…
```
These are time-limited pre-signed tokens. No decryption is required — a plain `curl` GET
downloads the file.

## Quick start

### With Docker

```bash
# Generate self-signed mTLS certificates
./scripts/gen-certs.sh

# Build and start
docker compose up --build -d

# Verify
curl --cacert certs/ca.crt https://localhost:8443/healthz
```

### As a CLI binary

```bash
go install github.com/deploymenttheory/go-sdk-windowsuup/cmd/winupdate@latest

# Discover current Windows 11 builds from Retail ring
winupdate fetch --arch amd64 --ring Retail

# Start the API server (plain HTTP for local dev)
winupdate serve --db winupdate.db --addr :8080
```

## CLI reference

### `winupdate fetch`

Performs a live SyncUpdates SOAP query and writes results to the catalog.

```
winupdate fetch [flags]

Flags:
  --arch          amd64 | arm64 | x86          (default: amd64)
  --ring          Retail | ReleasePreview | Beta | Dev | Canary  (default: Retail)
  --flight        Active | Skip | Current       (default: Active)
  --build         target build filter, e.g. "26100.8313" (empty = latest)
  --check-build   OS version the WU client claims to be on.
                  An old value causes WU to offer the current stable release
                  as an upgrade. (default: 10.0.16251.0)
  --sku           Windows SKU number (default 0 = Professional / SKU 48)
  --db            SQLite database path (default: winupdate.db)
```

Example — discover Windows 11 24H2 from Retail:

```bash
winupdate fetch --arch amd64 --ring Retail --check-build 10.0.16251.0
```

### `winupdate serve`

Starts the mTLS-protected REST API server.

```
winupdate serve [flags]

Flags:
  --addr          listen address (default: :8443)
  --db            SQLite database path (default: winupdate.db)
  --cert          server TLS certificate file
  --key           server TLS key file
  --ca            CA certificate file for mTLS client verification
  --watch-interval  background watcher poll interval (default: 30m)
  --no-watcher    disable background Windows Update watcher
```

## REST API

All `/v1/` routes require a valid client certificate when the server is started with `--cert`/`--ca`.
`/healthz` and `/readyz` are accessible without a client certificate.

| Method | Path | Description |
|---|---|---|
| `GET` | `/healthz` | Liveness probe |
| `GET` | `/readyz` | Readiness probe |
| `GET` | `/v1/builds` | List builds (filterable, paginated) |
| `GET` | `/v1/builds/:uuid` | Get a single build |
| `GET` | `/v1/builds/:uuid/files` | List files; add `?with_urls=true&revision=N` for CDN URLs |
| `GET` | `/v1/builds/:uuid/files/:name/download` | Stream file from Microsoft CDN |
| `GET` | `/v1/builds/:uuid/diff/:target` | Diff two builds' file sets |
| `POST` | `/v1/updates/fetch` | Trigger a live SyncUpdates query |
| `GET` | `/v1/feed` | Server-Sent Events change feed |

### Fetch a build (HTTP API)

```bash
curl --cert certs/client.crt --key certs/client.key --cacert certs/ca.crt \
     -X POST https://localhost:8443/v1/updates/fetch \
     -H 'Content-Type: application/json' \
     -d '{"arch":"amd64","ring":"Retail","check_build":"10.0.16251.0"}'
```

### Resolve CDN file URLs

```bash
UUID=<build-uuid>
REV=<revision>

curl --cert certs/client.crt --key certs/client.key --cacert certs/ca.crt \
     "https://localhost:8443/v1/builds/$UUID/files?with_urls=true&revision=$REV" \
     | jq '.data[] | {name, size_bytes, url}'
```

### Download a file directly from Microsoft CDN

```bash
CDN_URL=$(curl --cert certs/client.crt ... \
    "https://localhost:8443/v1/builds/$UUID/files?with_urls=true&revision=$REV" \
    | jq -r '.data[0].url')

curl -o windows11.esd "$CDN_URL"
```

Or via the REST API (server proxies the stream):

```bash
curl --cert certs/client.crt --key certs/client.key --cacert certs/ca.crt \
     -o windows11.esd \
     "https://localhost:8443/v1/builds/$UUID/files/windows11.esd/download?revision=$REV"
```

## ISO assembly

Once ESD/CAB files are downloaded they can be assembled into a bootable ISO on Linux or macOS
using standard open-source tools:

```bash
# Install prerequisites (Debian/Ubuntu)
sudo apt-get install cabextract wimtools chntpw genisoimage

# Install prerequisites (macOS with Homebrew)
brew tap sidneys/homebrew
brew install cabextract wimlib cdrtools sidneys/homebrew/chntpw
```

UUP dump's converter scripts (`uup_download_linux.sh`, `uup_download_macos.sh`) use these
tools to turn the downloaded ESD/CAB set into a bootable ISO.

## TLS certificates (development)

```bash
# Generate self-signed CA + server + client certs in ./certs/
./scripts/gen-certs.sh

# Call the API with mTLS
curl --cert certs/client.crt --key certs/client.key --cacert certs/ca.crt \
     https://localhost:8443/v1/builds
```

## Packages

| Package | Description |
|---|---|
| `wuproto` | Interface and domain types for the WU SOAP protocol |
| `wuproto/soap` | SOAP client: GetCookie → SyncUpdates → GetExtendedUpdateInfo2 |
| `catalog` | Domain types and Store/EventEmitter interfaces |
| `catalog/store` | SQLite implementation of catalog.Store |
| `catalog/watcher` | Background poller that keeps the catalog current |
| `catalog/events` | In-process event bus for build lifecycle events |
| `winupdate` | Service layer orchestrating SOAP + catalog |
| `api` | HTTP server setup and routing |
| `api/handlers` | Per-resource HTTP handlers |
| `api/middleware` | mTLS enforcement, logging, recovery |
| `sdk` | Thin Go client for the REST API |
| `cmd/winupdate` | CLI binary |

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-change`)
3. Commit your changes
4. Open a pull request against `main`

All submissions must pass `go build ./...` and `go vet ./...`.

## License

MIT License. See [LICENSE](LICENSE) for details.

## Disclaimer

This project is an independent implementation. It is not affiliated with, endorsed by, or
supported by Microsoft. Use in accordance with Microsoft's acceptable use policies.
