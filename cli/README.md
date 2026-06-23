# winmediafoundry CLI

A Cobra/Viper command-line tool over the whole toolkit: discover Windows Update
builds, browse the Media Creation Tool ESD catalog, inspect and extract WIM/ESD
images, and master bootable ISOs — all pure Go, no external tools.

## Install

```bash
# from a clone
go build -o winmediafoundry ./cli

# or install the binary (named "cli") onto your PATH
go install github.com/deploymenttheory/winmediafoundry/cli@latest
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
