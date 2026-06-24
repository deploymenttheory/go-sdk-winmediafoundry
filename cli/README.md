# winmediafoundry CLI

A Cobra/Viper command-line tool over the whole toolkit: discover Windows Update
builds, browse the Media Creation Tool ESD catalog, download consumer Windows 11
ISOs, inspect and extract WIM/ESD images, and master bootable ISOs — all pure Go,
no external tools.

## Install

```bash
# from a clone
go build -o winmediafoundry ./cli

# or install the binary (named "cli") onto your PATH
go install github.com/deploymenttheory/go-sdk-winmediafoundry/cli@latest
```

## Configuration

Settings are resolved with the precedence **flags > environment > config file**:

- **Config file**: `$HOME/.winmediafoundry.yaml` (or `--config <path>`).
- **Environment**: variables prefixed `WINMEDIAFOUNDRY_`, with `-` mapped to `_`
  (e.g. `WINMEDIAFOUNDRY_ARCH=arm64`, `WINMEDIAFOUNDRY_LOG_LEVEL=debug`).
- **Flags**: the global flags below, available on every command.

| Global flag | Default | Description |
|---|---|---|
| `--config` | `$HOME/.winmediafoundry.yaml` | config file path |
| `--timeout` | `2m` | HTTP request timeout for network commands |
| `--log-level` | `warn` | `debug`, `info`, `warn`, or `error` |
| `--arch` | `amd64` | CPU architecture (`amd64`, `x86`, `arm64`) |
| `--ring` | `Retail` | release ring (`Retail`, `Beta`, `ReleasePreview`, `Experimental`, `Canary`) |
| `--sku` | `pro` | SKU (`home`, `pro`, `enterprise`, `education`) |

Example config file:

```yaml
arch: arm64
ring: Beta
log-level: info
timeout: 5m
```

## Commands

### Windows Update

```bash
# list builds for the configured arch/ring/sku
winmediafoundry builds [--build 26100.4061]

# list a build's files (add --cdn-urls to resolve download URLs)
winmediafoundry files --build 26100.4061 --language en-us --edition Professional \
  --extension .esd --cdn-urls

# download a build's files to a directory
winmediafoundry download --build 26100.4061 --extension .esd --out ./dl --concurrency 4

# compare two builds
winmediafoundry diff --base 26100.3915 --target 26100.4061
```

### ESD catalog

```bash
winmediafoundry esd catalog --product windows11 \
  --edition Professional --architecture x64 --language en-us
```

### Consumer Windows 11 ISO (swdl)

`swdl` drives Microsoft's consumer software-download flow: it scrapes the public
Windows 11 ISO download pages, resolves a signed, time-limited link for a chosen
edition and language, and streams the multi-edition ISO to disk. This is distinct
from the Windows Update `download` command and the `esd` / `iso build` path — it
fetches the same consumer ISOs the browser download page serves.

```bash
# list the available editions (x64 and Arm64)
winmediafoundry swdl list [--architecture arm64]

# resolve a signed download link without downloading
# (argument is a product-edition id when all digits, otherwise a name substring)
winmediafoundry swdl resolve "Arm64" --language en-US

# download the ISO to a directory (re-runs skip an already-complete file)
winmediafoundry swdl download "Arm64" --out ./out --language en-US --progress
```

`swdl` flags (per subcommand):

| Flag | Applies to | Default | Description |
|---|---|---|---|
| `--architecture` | all | all | filter/select architecture (`x64` or `arm64`) |
| `--locale` | all | `en-US` | page/connector locale |
| `--language` | `resolve`, `download` | `English (United States)` | ISO language (e.g. `en-US` or the localized name) |
| `-o`, `--out` | `download` | `.` | destination directory |
| `--progress` | `download` | `true` | show a download progress bar |

### WIM / ESD images

```bash
winmediafoundry wim info ./install.esd
winmediafoundry wim tree ./install.esd --image 1 --depth 3
winmediafoundry wim extract ./install.esd --image 1 --out ./extracted
```

### Bootable ISO

```bash
winmediafoundry iso build ./install.esd ./Windows.iso --label CCCOMA_X64FRE
```

Run `winmediafoundry <command> --help` for full flag details.
