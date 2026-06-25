# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.6.0](https://github.com/deploymenttheory/go-sdk-winmediafoundry/compare/v0.5.0...v0.6.0) (2026-06-25)


### Features

* add support for injecting extra files into ISO media ([a735e22](https://github.com/deploymenttheory/go-sdk-winmediafoundry/commit/a735e22ceed585370dd61a27985deafd3d20a08e))

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
- `softwaredownload` — a standalone consumer software-download client
  (`softwaredownload.NewClient`), structured like `windowsuup`, that scrapes the
  Windows 11 ISO download pages, resolves signed download links, and streams the
  ISO to disk (`Get` / `List` / `GetByID` / `GetByName` / `Download`).
- `cli/` — a Cobra/Viper command-line tool (`winmediafoundry`) covering builds,
  files, download, diff, `esd catalog`, `swdl list|resolve|download`,
  `wim info|tree|extract`, and `iso build|inspect|extract-efi|fix-eltorito`,
  with layered flag / env (`WINMEDIAFOUNDRY_*`) / config-file settings.
- `swdl list|resolve|download` CLI commands exposing the `softwaredownload`
  client for downloading consumer Windows 11 ISOs.
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
