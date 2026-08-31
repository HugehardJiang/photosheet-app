# PhotoSheet Open Source Release Audit

Audit date: 2026-08-31

## Final gate

**OPEN SOURCE READINESS: READY — core source-only publication**

The candidate tree is technically clean for the public core workflow, has an explicit Apache-2.0 project license, and passes the tested build paths. The repository may be published as a source-only core project. Native RAW binaries, native SDK redistribution, installers and signed releases remain blocked until:

1. the exact Adobe DNG SDK archive, build metadata, license terms and bundled notices are manually reviewed;
2. the maintainer explicitly decides whether native RAW binaries may be redistributed;
3. target-platform validation is completed for any native binary, GUI installer or signed release.

The main project directory was deliberately not initialized as a Git repository, no remote was configured, and nothing was pushed.

## Scope and structure reviewed

The audit covered:

- Go CLI and application packages under cmd/ and internal/;
- Wails v2 GUI shell;
- React, TypeScript and Vite frontend;
- project schema, scanner, MaxRects packing, renderer, resolver and extractor;
- pure-Go DNG probe/decoder and optional LibRaw/DNG SDK/JPEG XL native bridge;
- updater;
- installer and build scripts;
- GitHub Actions;
- tests, screenshots, fixtures and generated output;
- local configuration, credentials and dependency manifests.

The public structure is now centered on cmd/, internal/, frontend/, docs/, native/, scripts/ and .github/. No old/new/final/backup source folders were found in the candidate tree.

## Cleanup and source hygiene

Generated and local-only content found during the initial scan was moved outside the project candidate tree so it remains recoverable for local development:

- root dist/ release binaries, app bundles and archives;
- Wails build/bin output;
- generated frontend dist/ and cmd/photosheet-gui/frontend/dist/;
- frontend/node_modules/, TypeScript build info and package checksum output;
- local macOS dylibs and Windows DLL;
- four .DS_Store files.

The candidate tree now contains no generated frontend output, dependency directory, OS metadata file, or native binary. The new .gitignore covers these classes explicitly. Native artifact paths are ignored exactly rather than all DLL/dylib files indiscriminately.

## 1. Secrets

**Secrets found: 0.**

Targeted scans found no API keys, tokens, bearer credentials, authorization headers, private-key blocks, passwords, signing credentials, cookies or webhook secrets. Credential filename scans found no real credential files; only the safe .env.example is present.

The CI workflow contains the literal regex vocabulary needed to detect ghp_ and similar patterns; those are scanner rules, not credentials, and the same regex was run against the temporary committed snapshot with no match.

gitleaks, trufflehog and gh were not installed, so no external secret scanner was introduced. The project had no .git directory or Git history at audit time; historical secret scanning is therefore not applicable. The first public history must still receive a final secret scan after the maintainer selects a license.

## 2. Personal paths and privacy

Private user paths found in old handoff documentation were replaced with generic examples. A second scan found no personal home path, school/company path, personal photo path, phone number or device-specific path in the candidate text.

Build scripts no longer depend on a developer-specific absolute SDK path. Native inputs are selected through PHOTOSHEET_* environment variables or relative project paths.

## 3. Photos and test media

No private photo, customer image, RAW/DNG fixture or ambiguous commercial media remains in the candidate tree.

The only retained raster assets are:

- cmd/photosheet-gui/build/appicon.png;
- docs/screenshots/pack-workspace-1024x700.png.

The screenshot was visually reviewed: it is a clean UI capture without private photos or private filesystem paths. Its size is approximately 42 KB.

The app icon is a simple monochrome monogram and contains no obvious third-party logo in a visual review. Original authorship and trademark clearance cannot be established from the repository alone; the maintainer should confirm those rights before publishing.

## 4. Large-file audit

After cleanup, the candidate tree contains no file larger than 5 MB, 20 MB, 50 MB or 100 MB.

The initial scan did find large generated or local-only files. They were not suitable for Git and were moved outside the candidate tree:

| Original path | Size | Decision |
| --- | ---: | --- |
| dist/PhotoSheet-windows-x64-portable.exe | 16.13 MB | generated artifact, exclude |
| bin/PhotoSheet.app/Contents/MacOS/PhotoSheet | 15.00 MB | generated artifact, exclude |
| dist/PhotoSheet-macos-arm64-portable.app/Contents/MacOS/PhotoSheet | 14.93 MB | generated artifact, exclude |
| dist/PhotoSheet-macos-arm64-portable.apk.app/Contents/MacOS/PhotoSheet | 14.93 MB | generated artifact, exclude |
| dist/归档.zip | 13.34 MB | local archive, exclude |
| dist/PhotoSheet.zip | 13.34 MB | local archive, exclude |
| dist/photosheet-macos-intel | 11.55 MB | generated artifact, exclude |
| dist/photosheet-windows-x64.exe | 10.97 MB | generated artifact, exclude |
| dist/photosheet-macos-arm64 | 9.69 MB | generated artifact, exclude |
| internal/raw/native/darwin/amd64/libphotosheet_raw.dylib | 8.26 MB | local native artifact, exclude pending review |
| internal/raw/native/windows/libraw.dll | 7.40 MB | local native artifact, exclude pending review |
| internal/raw/native/darwin/arm64/libphotosheet_raw.dylib | 6.57 MB | local native artifact, exclude pending review |
| dist/PhotoSheet-macos-arm64-portable.zip | 6.28 MB | generated artifact, exclude |

The frontend dependency directory also contained large tool binaries, but it is generated by npm ci and is ignored.

Git LFS was not added. These files are build outputs, local native inputs or test/development artifacts, not source-controlled assets.

## 5. Third-party dependency audit

Direct core dependencies are documented in THIRD_PARTY_NOTICES.md. Go and npm transitive dependencies remain tracked by go.sum and frontend/package-lock.json. No third-party SDK source or vendor tree is committed.

### LibRaw

The upstream LibRaw project describes the library as dual-licensed under LGPL-2.1 or CDDL 1.0. The optional native path must preserve the selected obligations, notices and any required corresponding-source offer. The project does not ship the LibRaw DLL/dylibs in the public candidate tree.

Source: [LibRaw About](https://www.libraw.org/about)

### Adobe DNG SDK

**NEEDS MANUAL LICENSE REVIEW.**

The local native lock entry records Adobe DNG SDK 1.7.1 build 2652 and a SHA-256 value, but the exact archive and complete notice bundle are not present in the repository. The official Adobe download page and the terms for the exact archive must be checked before any SDK source, bundled dependency, native binary or GitHub Release artifact is distributed. The build script now requires a local archive through PHOTOSHEET_NATIVE_DNG_SDK_ARCHIVE and does not auto-download the SDK.

Sources: [Adobe DNG SDK download page](https://helpx.adobe.com/camera-raw/digital-negative.html) and [Adobe DNG SDK EULA example](https://www.adobe.com/support/downloads/dng/dng_sdk_eula_mac.html)

### JPEG XL

The optional native path uses libjxl 0.11.2 as recorded by the local build metadata. The upstream license is BSD-3-Clause with the upstream PATENTS notice. Bundled Highway, Brotli, skcms, XMP Toolkit, libjpeg and zlib notices still need to be reconciled against the exact native SDK archive before redistribution.

Source: [libjxl license](https://github.com/libjxl/libjxl/blob/main/LICENSE)

### Other direct dependencies

- Wails v2.10.2: MIT.
- Go image and Go sys modules: BSD-3-Clause.
- React and React DOM: MIT.
- TypeScript: Apache-2.0.
- Vite and the React Vite plugin: MIT.

These core dependencies do not force a copyleft project license. They also do not override the separate Adobe SDK terms.

## 6. Project license

PhotoSheet is licensed under Apache-2.0. LICENSE identifies Copyright 2026 HugehardJiang. The project license does not grant permission to redistribute the separately licensed Adobe/native components.

The candidate analysis is in docs/LICENSE_OPTIONS.md:

- Apache-2.0 was selected because it provides an explicit patent grant and contribution terms;
- MIT remains a possible alternative for a future deliberate license change;
- MPL-2.0 or GPL-3.0 require a deliberate copyleft policy and do not solve proprietary SDK restrictions.

The license selection is complete for the source-only publication. A future license change would require a deliberate maintainer decision.

## 7. README and documentation

Completed:

- README.md — English entry point;
- README.zh-CN.md — Simplified Chinese entry point;
- BUILD_GUIDE.md;
- docs/ARCHITECTURE.md;
- docs/PROJECT_FORMAT.md;
- docs/RAW_PIPELINE.md;
- docs/RAW_BACKEND.md;
- docs/RELEASE.md;
- docs/LICENSE_OPTIONS.md;
- docs/OPEN_SOURCE_CHECKLIST.md;
- CONTRIBUTING.md;
- SECURITY.md;
- CODE_OF_CONDUCT.md;
- CHANGELOG.md;
- ROADMAP.md;
- THIRD_PARTY_NOTICES.md.

The README documents the supported JPG/JPEG, PNG and TIFF baseline; the constrained pure-Go DNG path; Strict Lossless semantics; the RAW-to-RGB boundary; GUI and CLI workflows; build commands; platform prerequisites; optional native flags; privacy boundaries; Beta status; and release limitations.

The README explicitly says that JPEG output is re-encoded and that Quality 100 is not mathematically lossless. It also distinguishes RAW Source Integrity from RGB Pixel Integrity.

## 8. Contributor experience

CONTRIBUTING.md covers fork/branch/PR flow, Go formatting, context.Context, error wrapping, strict TypeScript, focused tests, packing no-overlap tests, pixel round trips, RAW fixture policy and installer/updater checks. Issue and pull-request templates require platform, format, bit depth, RAW backend, diagnostics and schema-impact information without asking users to expose private RAW files.

## 9. GitHub Actions and automation

Completed:

- .github/workflows/ci.yml — Go tests/vet, frontend install/lint/typecheck/build, Wails compile check, cross-platform core CLI builds and targeted secret scan;
- .github/workflows/codeql.yml — Go and JavaScript/TypeScript analysis;
- .github/workflows/release.yml — vMAJOR.MINOR.PATCH tag workflow for unsigned core CLI/updater artifacts and SHA256SUMS;
- .github/dependabot.yml — weekly Go, npm and GitHub Actions updates;
- .github/ISSUE_TEMPLATE/bug_report.yml;
- .github/ISSUE_TEMPLATE/feature_request.yml;
- .github/pull_request_template.md.

The release workflow does not require signing secrets and does not publish native binaries, installers or notarized packages. Native and installer workflows remain deferred until legal and platform review.

SBOM generation was not added in this pass because the release scope is intentionally limited to the public core artifacts; it remains a reasonable future release enhancement.

## 10. Build reproducibility

The default public build uses the pure-Go RAW fallback and does not need LibRaw, Adobe DNG SDK, JPEG XL, certificates or notarization credentials. Native RAW support is opt-in through photosheet_native and requires reviewed local inputs.

The macOS native build script verifies the LibRaw archive hash, requires an explicitly supplied DNG SDK archive, verifies its recorded hash, and rejects disallowed Homebrew/local dynamic dependencies. It no longer silently downloads the Adobe SDK.

The environment scripts are scripts/check-env.ps1 and scripts/check-env.sh. The PowerShell script was executed successfully. Bash was unavailable on the audit host, so shell syntax was not checked locally.

## 11. Test and platform results

See TEST_REPORT.md for the command-level record. Summary:

- Go tests: passed.
- Go vet: passed.
- Frontend lint: passed.
- Frontend typecheck: passed.
- Frontend production build: passed.
- Wails frontend asset generation: passed.
- Wails GUI compile check on Windows: passed.
- Windows core CLI build: passed.
- Linux core CLI cross-builds for amd64/arm64: passed.
- macOS core CLI cross-builds for amd64/arm64: passed.
- Fresh-clone core, frontend and GUI compile verification: passed.
- Native LibRaw/Adobe DNG SDK/JPEG XL build: not run; blocked by platform and license review.
- Installer/signing/notarization: not run.

## 12. Requested final answers

| # | Question | Result |
| ---: | --- | --- |
| 1 | Secret discovered? | No. Targeted scan result: 0. |
| 2 | Personal path discovered? | Old private documentation paths were scrubbed; final scan: 0. |
| 3 | Private photos discovered? | No private photos or RAW/DNG fixtures retained. |
| 4 | Large files discovered? | Initial generated/native files over 5 MB were moved out; final candidate: 0 files over 5 MB. |
| 5 | Third-party dependencies that cannot be directly committed? | Adobe DNG SDK and its bundled native source/artifacts remain uncommitted pending review; local native binaries remain ignored. |
| 6 | LibRaw license result? | LGPL-2.1 or CDDL 1.0 according to upstream; notices and redistribution obligations required. |
| 7 | DNG SDK license result? | NEEDS MANUAL LICENSE REVIEW; no SDK source/binary release allowed yet. |
| 8 | JPEG XL dependency license result? | libjxl is BSD-3-Clause plus PATENTS; bundled dependency notices still need exact-archive reconciliation. |
| 9 | PhotoSheet current license? | Apache-2.0, Copyright 2026 HugehardJiang. |
| 10 | License change recommended? | No change needed for this source-only release; native dependencies remain separately governed. |
| 11 | README complete? | Yes, English README completed. |
| 12 | Chinese README complete? | Yes, README.zh-CN.md completed. |
| 13 | CONTRIBUTING complete? | Yes. |
| 14 | SECURITY complete? | Yes, using GitHub Security Advisories without inventing an email. |
| 15 | CI complete? | Core CI, GUI compile, platform CLI matrix, secret scan and CodeQL definitions completed. |
| 16 | Release workflow complete? | Unsigned core CLI/updater tag workflow completed; installers/native/signing are deferred. |
| 17 | Fresh Clone Build passed? | Yes, including core Go, frontend, Wails asset generation and GUI compile. |
| 18 | Windows build passed? | Core CLI cross-build and Windows-host Wails compile passed; installer/signing not run. |
| 19 | macOS build passed? | Core CLI cross-build passed; macOS GUI/native RAW build not run. |
| 20 | Linux build passed? | Core CLI cross-build passed; Linux GUI build deferred to CI. |
| 21 | RAW build passed? | Pure-Go supported DNG tests passed; native RAW build not run. |
| 22 | All tests passed? | All executed core/frontend/GUI checks passed; deferred native/platform packaging checks are explicitly not claimed. |
| 23 | Fully ready for public GitHub? | Yes for the source-only core repository; native binary/release distribution remains blocked pending review. |

## 13. GitHub publication preparation

The target is now confirmed as HugehardJiang/photosheet-app, Public. No GitHub remote has been configured yet. Before the native or installer blockers are resolved, the maintainer should:

- keep the selected LICENSE in the initial commit;
- rerun the final secret/media/large-file audit;
- initialize Git with a clean initial commit such as Initial open-source release;
- use main as the default branch unless an existing release convention requires otherwise;
- configure a confirmed remote without force push;
- enable Issues, Dependabot alerts, Security Advisories and secret scanning where available;
- protect main with pull requests, required CI and no force pushes;
- use a concise repository description such as: Cross-platform tool for packing photos into processing sheets and restoring them with pixel-accurate coordinates.

Suggested topics: photosheet, photography, image-processing, batch-processing, raw, dng, wails, golang, desktop, cross-platform.

An unrelated archived GitHub repository already uses the name Photosheet for a Blogger template. The software name need not change, but photosheet-app is a reasonable repository-name fallback if the desired owner cannot use photosheet.
