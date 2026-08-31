# PhotoSheet test report

Audit date: 2026-08-31

## Environment

- Windows amd64 host.
- Go 1.26.5, supplied from an isolated audit toolchain because Go was not on the original PATH.
- Node.js v24.17.0 and npm 11.13.0.
- Git 2.53.0.windows.2.

## Passed checks

The following commands completed successfully in the working tree:

~~~text
go test ./...
go vet ./...
go build -trimpath ./cmd/photosheet
go build -trimpath ./cmd/photosheet-updater

cd frontend
npm ci
npm run lint
npm run typecheck
npm run build
npm run build:wails

cd ..
go test -tags wails ./cmd/photosheet-gui
~~~

The core CLI also cross-compiled successfully with CGO disabled for:

- Linux amd64 and arm64;
- macOS amd64 and arm64;
- Windows amd64 and arm64.

## Fresh-clone verification

A temporary Git repository was created outside the project, committed, cloned into a second clean directory, and tested using the documented core workflow. The clean clone passed:

- Go core tests;
- frontend dependency installation;
- frontend lint, typecheck and production build;
- Wails frontend asset generation;
- Wails GUI package compilation;
- final Git status check.

The fresh clone tracked no native binaries and no generated frontend output, and its final Git status was clean.

## Not run or intentionally deferred

- Native LibRaw/Adobe DNG SDK/JPEG XL builds: not run. They require target macOS/Windows environments and a manual license review of the exact Adobe SDK archive.
- Native RAW integration tests: not run because no legally distributable fixture is present in the repository.
- macOS GUI and installer build: not run on a macOS host.
- Linux Wails GUI build: not run locally; the CI workflow contains a separate Linux Wails compile job.
- Installer packaging, signing and notarization: intentionally excluded from the current unsigned core release workflow.
- Race tests: not run; enabling CGO requires a C compiler, and gcc is not installed on the audit host.
- Shell syntax checks: not run locally because bash is unavailable in this Windows environment.
- YAML parser check: not run locally because Ruby/PyYAML is unavailable; GitHub Actions will validate the workflow when executed.
- Third-party secret scanners such as gitleaks/trufflehog: not installed; targeted repository scans were run instead.

## Interpretation

The public core source and GUI compile path are reproducible from a clean clone. The source-only core repository is ready for publication under Apache-2.0. The optional native redistribution boundary remains under manual review and is excluded from the repository artifacts and current release workflow.
