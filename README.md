# PhotoSheet

PhotoSheet is a cross-platform desktop tool for packing photos into processing sheets and restoring the edited sheets to individual images using the project metadata.

**Status: Beta.** The core JPG/PNG/TIFF workflow is the supported public baseline. RAW/DNG decoding is deliberately split into a self-contained core fallback and an optional native backend; see [RAW support](docs/RAW_PIPELINE.md).

[简体中文](README.zh-CN.md)

## Overview

```text
Photos
  ↓
PhotoSheet Pack
  ↓
Processing sheets + project.json
  ↓
External image editor
  ↓
Processed sheets
  ↓
PhotoSheet Restore
  ↓
Individual images
```

PhotoSheet keeps source files read-only. It records stable sheet identities, image coordinates, dimensions, orientation and hashes in `project.json`, then uses those records to resolve processed sheets even when they are renamed or stored in different directories.

## Features

- JPG/JPEG, PNG and TIFF input scanning.
- DNG metadata probing and a constrained pure-Go decoder for uncompressed and Deflate DNGs.
- PNG and TIFF sheets, including the 16-bit pipeline.
- Strict Lossless mode and an explicitly separate JPEG Compatibility mode.
- Deterministic MaxRects packing with optional 90° rotation and padding.
- `project.json` v3 with migration support for older projects.
- Processed-sheet resolution by canonical stem, safe suffix matching, dimensions and manual bindings.
- PNG/JPEG/TIFF extraction with path-traversal and unsafe-transform checks.
- CLI and Wails v2 + React/TypeScript GUI using the same Go application layer.
- Optional updater with signed manifest and SHA-256 verification.

Native LibRaw/Adobe DNG SDK/JPEG XL support is not part of the default public build. The local native artifacts currently present in some development checkouts are ignored by Git until their redistribution terms, notices and source-offer obligations have been reviewed. Do not describe them as public release artifacts.

## What “Strict Lossless” means

In Strict Lossless mode, PhotoSheet decodes an input image and preserves the resulting RGB pixels while packing into and extracting from PNG or lossless TIFF sheets. It does not resize, sharpen or apply a lossy color conversion during that round trip.

This does **not** restore information already lost by a JPEG source. A JPEG output is always re-encoded; `Quality=100` is not mathematically lossless. The same boundary applies to RAW: demosaicing and white balance convert sensor data into a working RGB image, so RAW source integrity and RGB pixel integrity are different guarantees.

## RAW/DNG boundary

The conceptual pipeline is:

```text
DNG / RAW → probe → decode → demosaic → white balance → working color space → 16-bit RGB → sheet
```

The pure-Go fallback is suitable for the supported uncompressed/Deflate DNG subset. JPEG-compressed DNG, lossy JPEG DNG, JPEG XL DNG and camera-specific RAW variants require a separately reviewed native backend. A native backend, when legally configured by a maintainer, must output display-oriented 16-bit RGB; it does not preserve the original sensor structure through the sheet workflow.

See [RAW_PIPELINE.md](docs/RAW_PIPELINE.md), [RAW_BACKEND.md](docs/RAW_BACKEND.md) and [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

## Quick start

1. Select photos or a folder.
2. Set the maximum sheet dimensions and output format.
3. Generate sheets and `project.json`.
4. Process the sheets in an external image editor.
5. Select the project and the processed sheet files or directories.
6. Review automatic matches; resolve ambiguous candidates manually.
7. Restore individual images. Keep the originals as a backup.

The GUI stores recent processed locations in the user configuration directory, not in `project.json`.

## Build the core project

The reproducible public baseline requires:

- Go 1.22 or newer;
- Node.js 22 LTS or newer and npm;
- Git.

From a fresh clone:

```bash
go mod download
go test ./...
go vet ./...

cd frontend
npm ci
npm run typecheck
npm run build
cd ..

mkdir dist
go build -trimpath -o dist/photosheet ./cmd/photosheet
go build -trimpath -o dist/photosheet-updater ./cmd/photosheet-updater
```

Run the CLI with `photosheet --help`, or inspect a project with `photosheet verify --project ./path/to/project.json`.

### GUI development and build

PhotoSheet uses Wails v2.10.2. The GUI needs the native WebView/toolchain for the target platform.

```bash
cd frontend
npm ci
npm run build:wails
cd ..
go test -tags wails ./cmd/photosheet-gui

cd cmd/photosheet-gui
go run github.com/wailsapp/wails/v2/cmd/wails@v2.10.2 dev
go run github.com/wailsapp/wails/v2/cmd/wails@v2.10.2 build -clean -tags "wails production"
```

Platform-specific GUI prerequisites are documented in [BUILD_GUIDE.md](BUILD_GUIDE.md). Core builds do not require LibRaw, Adobe DNG SDK, JPEG XL, a signing certificate or a notarization credential.

### Optional native RAW build

The native backend is intentionally opt-in and is not downloaded by CI. It requires a reviewed local LibRaw/DNG SDK build artifact at the paths described in [RAW_BACKEND.md](docs/RAW_BACKEND.md). Use `-tags photosheet_native` only after confirming that every SDK license permits the intended source/binary distribution and that the required notices are available.

```text
Windows x64: internal/raw/native/windows/libraw.dll
macOS arm64: internal/raw/native/darwin/arm64/libphotosheet_raw.dylib
macOS x86_64: internal/raw/native/darwin/amd64/libphotosheet_raw.dylib
```

These paths are ignored by Git. Linux and Windows ARM64 use the public fallback unless a separate, reviewed backend is supplied.

## Architecture

```text
React/TypeScript frontend
          ↓ Wails bindings
Go application layer
          ↓
Scanner · Packing · Renderer · Resolver · Extractor
          ↓
PNG/JPEG/TIFF codecs · project schema · optional RAW backend
```

The frontend owns UI, interaction and preview. The Go packages own file inspection, packing, rendering, project validation and extraction. This repository is an application, not a stable public Go SDK; packages under `internal/` are not an external API.

More detail is in [ARCHITECTURE.md](docs/ARCHITECTURE.md) and [PROJECT_FORMAT.md](docs/PROJECT_FORMAT.md).

## Privacy and safety boundaries

- Pack and restore image processing is local; the updater is the only network-facing feature.
- Source images are opened read-only and are not modified, moved or deleted by the core workflow.
- `project.json` is data, not a script; PhotoSheet does not execute commands from it.
- The resolver rejects path traversal, symlink escapes, invalid image formats, overlapping rectangles and non-proportional transforms.
- External editors may change resolution, bit depth, ICC data, compression or canvas geometry. Keep the originals and review warnings before restoring.

Do not upload private photos, RAW files or project files to an issue unless you have the rights to share them. Prefer metadata, diagnostics and a minimal generated fixture.

## License

PhotoSheet is licensed under the Apache License 2.0. See [LICENSE](LICENSE). Third-party terms are summarized in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md); they do not grant a license to PhotoSheet itself. Optional native RAW artifacts remain excluded until their separate redistribution review is complete.

## Contributing and security

Please read [CONTRIBUTING.md](CONTRIBUTING.md), [SECURITY.md](SECURITY.md) and [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) before opening a pull request or reporting a vulnerability.

## Release status

The tag-driven workflow currently publishes unsigned core CLI/updater artifacts and `SHA256SUMS`. Native RAW binaries, installers, signing and notarization remain separate release decisions until the license review and platform validation are complete. See [RELEASE.md](docs/RELEASE.md).
