# PhotoSheet build guide

This guide covers the reproducible public core build. It intentionally separates it from the optional native RAW build.

## Prerequisites

All platforms:

- Go 1.22 or newer;
- Node.js 22 LTS or newer;
- npm and Git.

GUI builds additionally require Wails v2.10.2 and the native WebView toolchain:

- Windows: WebView2 Runtime, a C/C++ toolchain and NSIS only when making an NSIS installer.
- macOS: Xcode Command Line Tools. A signed/notarized package also needs Apple signing identities.
- Debian/Ubuntu Linux: build-essential, pkg-config, GTK 3 development files and the WebKitGTK development package available for the distribution.

For a core CLI build, no C/C++ compiler, LibRaw, Adobe DNG SDK or JPEG XL installation is needed.

## Core build

~~~bash
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
~~~

Use photosheet --help to inspect the CLI. The build writes only to dist, which is ignored by Git.

## GUI build

Generate Wails assets and compile the GUI on the target operating system:

~~~bash
cd frontend
npm ci
npm run build:wails
cd ../cmd/photosheet-gui
go run github.com/wailsapp/wails/v2/cmd/wails@v2.10.2 dev
go run github.com/wailsapp/wails/v2/cmd/wails@v2.10.2 build -clean -tags "wails production"
~~~

The source frontend is frontend/. cmd/photosheet-gui/frontend/dist is generated and ignored. The GUI can be built without the native RAW backend; unsupported native capabilities are reported by diagnostics.

## Optional native build

Native RAW support is a separate, maintainer-only build. It requires reviewed local artifacts and the photosheet_native build tag. Follow docs/RAW_BACKEND.md and native/README.md. Do not download or redistribute the Adobe DNG SDK from an unreviewed automation path.

## Cross-platform CLI release build

scripts/build-release.sh creates fallback CLI/updater binaries for six target combinations. It uses VERSION for the application version and PHOTOSHEET_NATIVE_MACOS=1 or PHOTOSHEET_NATIVE_WINDOWS=1 only for explicitly reviewed local native inputs. The public GitHub workflow does not set those flags.

Windows GUI can be built with scripts/build-windows-portable.ps1. Passing -NativeRaw is opt-in and requires a reviewed local Windows amd64 artifact. macOS GUI can be built with scripts/build-gui-macos.sh; it defaults to core mode and accepts PHOTOSHEET_NATIVE_MACOS=1 only when the local Darwin artifact has been reviewed.
