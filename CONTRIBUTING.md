# Contributing to PhotoSheet

Thank you for helping improve PhotoSheet. The project is currently Beta, and the core file format and image workflow are more stable than the GUI presentation layer.

## Before you start

Please search existing issues before opening a new one. For security vulnerabilities, follow SECURITY.md instead of opening a public issue. Do not upload private photos, RAW files, customer material or project files unless you have explicit redistribution rights.

## Development setup

Install Go 1.22+, Node.js 22 LTS+, npm and Git. Then run:

~~~bash
go mod download
go test ./...
go vet ./...
cd frontend
npm ci
npm run typecheck
npm run build
~~~

The Wails frontend used by the GUI is generated with npm run build:wails. Wails v2.10.2 and target-platform prerequisites are described in BUILD_GUIDE.md.

## Code style

- Run gofmt on changed Go files.
- Prefer idiomatic Go, context.Context for cancellable work and wrapped errors with useful context.
- Keep image operations bounded by the existing validation and memory/tile mechanisms.
- Keep TypeScript strict and avoid any. Reuse existing components and do not add a large framework for a small problem.
- Keep project schema changes explicit and update docs/PROJECT_FORMAT.md.
- Do not introduce absolute developer paths, generated bundles, local configuration or test media into source control.

## Tests

Every behavior change should include or update a focused test.

- Go: go test ./... and go vet ./...
- Frontend: npm run typecheck and npm run build
- Packing: test that rectangles do not overlap and remain inside the canvas.
- Strict Lossless: run a pixel round-trip test when pixel handling changes.
- Resolver: cover renamed/suffixed results, different directories and ambiguity.
- RAW: add an integration test only with a fixture that may legally be redistributed; otherwise document the local fixture requirement and keep it out of Git.
- Installer/updater: test signature, hash, path and rollback behavior.

Optional native RAW tests require the photosheet_native build tag and a reviewed local artifact. They are not part of the ordinary public CI.

## Branches and pull requests

1. Fork the repository and create a focused branch from main.
2. Make the smallest coherent change.
3. Run the relevant tests and record the exact commands in the pull request.
4. Update user or contributor documentation when behavior changes.
5. Open a pull request using the template and explain platform-specific impact.

Pull requests that change project schema, RAW behavior, lossless guarantees, installers or updater behavior must call that out explicitly. Screenshots should contain only generated or openly licensed material.

## Commits

Use a short imperative subject, keep unrelated changes separate, and do not commit build output, secrets, private paths, logs or large fixtures. A conventional-commit format is welcome but not required.
