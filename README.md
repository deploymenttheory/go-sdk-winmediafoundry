[![Go Report Card](https://goreportcard.com/badge/github.com/deploymenttheory/go-sdk-windowsuup)](https://goreportcard.com/report/github.com/deploymenttheory/go-sdk-windowsuup)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/github/go-mod/go-version/deploymenttheory/go-sdk-windowsuup)](https://github.com/deploymenttheory/go-sdk-windowsuup)
![Status: Experimental](https://img.shields.io/badge/status-experimental-orange)

## Getting Started with `go-sdk-windowsuup`

`go-sdk-windowsuup` is a pure Go client library for Microsoft's Windows Update SOAP protocol. It makes direct SOAP calls to `fe3.delivery.mp.microsoft.com` and `fe3cr.delivery.mp.microsoft.com`. Use it to discover available Windows builds by ring and architecture, resolve pre-signed CDN download URLs, stream ESD/CAB files to disk, and compare file sets between builds.

## Go Prerequisites

- Go 1.23 or later
- Outbound HTTPS (port 443) to `fe3.delivery.mp.microsoft.com` and `fe3cr.delivery.mp.microsoft.com`
- No certificates required - the SDK handles the Microsoft CA chain for you.

## Installation

```bash
go get github.com/deploymenttheory/go-sdk-windowsuup
```

## Usage

Runnable examples are in the `examples/` directory:

| Example | Description |
|---|---|
| `examples/01_fetch_builds` | Discover available Windows builds by ring and architecture |
| `examples/02_get_files` | Resolve pre-signed CDN download URLs for a build's files |
| `examples/03_download` | Stream ESD/CAB files concurrently to a local directory |
| `examples/04_diff` | Compare file sets between two builds |

Run any example directly:

```bash
go run ./examples/01_fetch_builds
```

## Creating a Client

```go
import "github.com/deploymenttheory/go-sdk-windowsuup/windowsuup"

client, err := windowsuup.NewClient()
if err != nil {
    log.Fatal(err)
}
```

`NewClient` accepts zero or more `ClientOption` values:

| Option | Type | Default | Description |
|---|---|---|---|
| `WithTimeout(d)` | `time.Duration` | 2 min | Per-SOAP-request HTTP timeout; CDN downloads are exempt |
| `WithTLSConfig(cfg)` | `*tls.Config` | embedded Microsoft CA bundle + system roots | Custom TLS configuration for SOAP connections |
| `WithHTTPClient(hc)` | `*http.Client` | internal | Replace the underlying HTTP client for SOAP calls; overrides `WithTimeout` and `WithTLSConfig` |
| `WithLogger(l)` | `*zap.Logger` | `zap.NewProduction()` | Structured logger |

Example with options:

```go
import (
    "time"
    "go.uber.org/zap"

    "github.com/deploymenttheory/go-sdk-windowsuup/windowsuup"
)

logger, _ := zap.NewDevelopment()
client, err := windowsuup.NewClient(
    windowsuup.WithTimeout(60 * time.Second),
    windowsuup.WithLogger(logger),
)
```

## Calling SDK Functions

### Fetch Builds

`client.Builds.FetchBuilds` calls the SyncUpdates SOAP endpoint and returns all available builds matching the given filters.

```go
import (
    buildsapi "github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/api/builds"
    "github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/constants"
)

builds, _, err := client.Builds.FetchBuilds(ctx,
    buildsapi.WithArch(constants.ArchAMD64),
    buildsapi.WithRing(constants.RingRetail),
    buildsapi.WithSKU(constants.SKUPro),
)
```

`FetchOption` reference:

| Option | Default | Description |
|---|---|---|
| `WithArch(arch)` | `ArchAMD64` | Target CPU architecture |
| `WithRing(ring)` | `RingRetail` | Windows Update release channel |
| `WithSKU(sku)` | `SKUPro` | Windows edition SKU |
| `WithFlight(flight)` | `"Active"` | Update flight sub-channel |
| `WithCheckBuild(build)` | `""` (SDK default) | OS version the client claims to run; an old value causes WU to offer the current release as an upgrade |
| `WithBuild(build)` | `""` | Filter to a specific build version string, e.g. `"26100.4061"` |

Each `models.Build` in the result includes `UUID`, `Revision`, `Title`, `Build` (version string), `Arch`, `Ring`, `IsStable`, and `IsInsider`.

### Get Files

`client.Files.GetFiles` retrieves the file list for a build. Without `WithCDNURLs`, it returns file metadata only (SHA1, SHA256, size). With `WithCDNURLs`, it calls GetExtendedUpdateInfo2 to resolve pre-signed Microsoft CDN download URLs that expire approximately 12 minutes after resolution.

```go
import (
    filesapi "github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/api/files"
    "github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/constants"
)

files, _, err := client.Files.GetFiles(ctx, build,
    filesapi.WithCDNURLs(),
    filesapi.WithLanguage("en-us"),
    filesapi.WithEdition(constants.EditionProfessional),
    filesapi.WithExtension(".esd"),
)
```

`FileOption` reference:

| Option | Description |
|---|---|
| `WithCDNURLs()` | Resolve live pre-signed CDN download URLs (expire ~12 min) |
| `WithLanguage(lang)` | Filter by BCP-47 language tag, e.g. `"en-us"`. Neutral files are always included. |
| `WithEdition(ed)` | Filter by Windows edition using filename substring matching |
| `WithExtension(ext)` | Filter by file extension, e.g. `".esd"` or `".cab"` |

Each `models.File` in the result includes `Name`, `SizeBytes`, `SHA1`, `SHA256`, `FileType`, and — when `WithCDNURLs` is set — `URL` and `ExpiresAt`.

### Download Files

`client.Download.DownloadFile` streams a single file from its CDN URL to a destination directory. `client.Download.DownloadFiles` downloads multiple files concurrently. Files are written atomically (temp file → rename); files already present at the correct size are skipped.

Both methods require files with a populated `URL` field — call `GetFiles` with `WithCDNURLs()` first.

```go
// Single file
resp, err := client.Download.DownloadFile(ctx, files[0], "./downloads")

// Multiple files — concurrency=0 defaults to 4 parallel downloads
err := client.Download.DownloadFiles(ctx, files, "./downloads", 4)
```

### Diff Builds

`client.Diff.Diff` compares the file sets of two builds client-side. It fetches file metadata for both builds (no CDN URLs) and compares by SHA256, falling back to SHA1, then size. The result reports files added, removed, changed, and unchanged.

```go
d, _, err := client.Diff.Diff(ctx, buildA, buildB)
fmt.Printf("+%d -%d ~%d =%d\n",
    len(d.Added), len(d.Removed), len(d.Changed), d.Unchanged)
```

`models.BuildDiff` fields:

| Field | Type | Description |
|---|---|---|
| `BaseUUID` / `TargetUUID` | `string` | Build UUIDs being compared |
| `BaseBuild` / `TargetBuild` | `string` | Build version strings |
| `Added` | `[]models.File` | Files present in target but not in base |
| `Removed` | `[]models.File` | Files present in base but not in target |
| `Changed` | `[]models.FileDiff` | Files present in both but with differing content |
| `Unchanged` | `int` | Count of files identical in both builds |

## Constants Reference

All constants live in `github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/constants`.

### Architectures

| Constant | Value |
|---|---|
| `ArchAMD64` | `"amd64"` |
| `ArchX86` | `"x86"` |
| `ArchARM64` | `"arm64"` |

### Rings

| Constant | Channel |
|---|---|
| `RingRetail` | Stable / generally available |
| `RingReleasePreview` | Release Preview Insider |
| `RingBeta` | Beta Insider |
| `RingExperimental` | Dev Insider |
| `RingCanary` | Canary Insider (fastest-moving) |
| `RingMSIT` | Microsoft internal |

`RingDev` is a deprecated alias for `RingExperimental`.

### SKUs

| Constant | Name | ID |
|---|---|---|
| `SKUPro` | Professional | 48 |
| `SKUHome` | Home | 1 |
| `SKUHomeN` | Home N | 2 |
| `SKUEnterprise` | Enterprise | 4 |
| `SKUEducation` | Education | 121 |
| `SKUProWorkstation` | Pro for Workstations | 161 |
| `SKUIoTEnterprise` | IoT Enterprise | 188 |
| `SKUServerStandard` | Server Standard | 7 |
| `SKUServerDatacenter` | Server Datacenter | 8 |

### Editions

Used with `filesapi.WithEdition(ed)` to filter files by Windows edition.

| Constant | Edition |
|---|---|
| `EditionHome` | Home (CORE) |
| `EditionHomeN` | Home N |
| `EditionProfessional` | Professional |
| `EditionProfessionalN` | Professional N |
| `EditionEnterprise` | Enterprise |
| `EditionEnterpriseN` | Enterprise N |
| `EditionEducation` | Education |
| `EditionEducationN` | Education N |
| `EditionProWorkstation` | Pro for Workstations |
| `EditionServerStandard` | Server Standard |
| `EditionServerDatacenter` | Server Datacenter |

## ISO Assembly

Once ESD/CAB files are downloaded they can be assembled into a bootable ISO on Linux or macOS using standard open-source tools:

```bash
# Install prerequisites (Debian/Ubuntu)
sudo apt-get install cabextract wimtools chntpw genisoimage

# Install prerequisites (macOS with Homebrew)
brew tap sidneys/homebrew
brew install cabextract wimlib cdrtools sidneys/homebrew/chntpw
```

UUP dump's converter scripts (`uup_download_linux.sh`, `uup_download_macos.sh`) use these tools to turn the downloaded ESD/CAB set into a bootable ISO.

## Package Layout

| Package | Description |
|---|---|
| `windowsuup` | Entry point — `Client`, `NewClient`, `ClientOption` |
| `windowsuup/api/builds` | `FetchBuilds` — build discovery via SyncUpdates SOAP |
| `windowsuup/api/files` | `GetFiles` — file metadata and CDN URL resolution via GetExtendedUpdateInfo2 |
| `windowsuup/api/download` | `DownloadFile` / `DownloadFiles` — streaming CDN downloads |
| `windowsuup/api/diff` | `Diff` — client-side file-set comparison |
| `windowsuup/constants` | `Arch`, `Ring`, `SKU`, `Edition` constants |
| `windowsuup/shared/models` | `Build`, `File`, `BuildDiff`, `FileDiff` types |
| `windowsuup/client` | Transport interface and concrete `Transport` implementation |
| `internal/wuproto` | Windows Update SOAP protocol types (internal) |
| `internal/wuproto/soap` | SOAP client: GetCookie → SyncUpdates → GetExtendedUpdateInfo2 |

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-change`)
3. Commit your changes
4. Open a pull request against `main`

All submissions must pass `go build ./...` and `go vet ./...`.

## License

MIT License. See [LICENSE](LICENSE) for details.

## Disclaimer

This project is an independent implementation. It is not affiliated with, endorsed by, or supported by Microsoft. Use in accordance with Microsoft's acceptable use policies.
