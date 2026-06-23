# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Pure-Go Windows imaging libraries under `pkg/` (no cgo, no external tools):
  - `pkg/wim` — read, extract, and write WIM/ESD images (container, blob table,
    XML catalog, solid LZMS resources, dentry tree, extraction, multi-image
    writer, and direct WIM→WIM image copy).
  - `pkg/wim/lzms` and `pkg/wim/xpress` — LZMS and XPRESS decompressors; the
    reader now handles all four WIM formats (none/XPRESS/LZX/LZMS).
  - `pkg/cab` — Microsoft Cabinet reader with LZX and MSZIP decompression.
  - `pkg/udf` — UDF 1.02 (ECMA-167) writer.
  - `pkg/iso` — bootable ISO9660 + El Torito mastering and the UDF + El Torito
    bridge master.
  - `pkg/builder` — end-to-end ESD → bootable ISO orchestration (`BuildISO`).
- `esd` — a standalone Media Creation Tool ESD catalog client (`esd.NewClient`),
  structured like `windowsuup` with its own transport, models, and mocks.
- `cli/` — a Cobra/Viper command-line tool (`winmediafoundry`) covering builds,
  files, download, diff, `esd catalog`, `wim info|tree|extract`, and `iso build`,
  with layered flag / env (`WINMEDIAFOUNDRY_*`) / config-file settings.
- Examples `05_esd_catalog` through `09_esd_to_iso`.

### Changed

- Moved the imaging libraries from `windowsuup/{wim,iso,udf,builder}` and
  `windowsuup/internal/cab` to top-level `pkg/{wim,iso,udf,builder,cab}`; they are
  general-purpose and do not depend on the Windows Update service client.
- Moved the WU SOAP protocol layer from `internal/wuproto` to `pkg/wuproto`
  (now a public, reusable package).
- Split the ESD catalog out of the `windowsuup` client into a separate top-level
  `esd` client; it is no longer exposed as `windowsuup.Client.ESD`.

### Fixed

- WIM reader: a zero FILETIME (unset timestamp) decoded to a year-1601 time whose
  `UnixNano` overflowed int64; it now maps to the zero `time.Time`.

## [1.1.0] - 2021-06-23

### Added

- Added x [@your_username](https://github.com/your_username)

### Changed

- Changed y [@your_username](https://github.com/your_username)

## [1.0.0] - 2021-06-20

### Added

- Inititated y [@your_username](https://github.com/your_username)
- Inititated z [@your_username](https://github.com/your_username)