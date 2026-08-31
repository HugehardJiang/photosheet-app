# macOS native backend handoff

This file is a public, reproducible handoff checklist for maintainers working on the optional macOS RAW backend. It does not certify a release.

## Required handoff data

Record:

- macOS version and machine architecture;
- Xcode/AppleClang, macOS SDK, CMake, Ninja, Go and Node versions;
- exact LibRaw, Adobe DNG SDK, libjxl, XMP and libjpeg inputs;
- source archive hashes and final dylib hashes;
- file, exported-symbol and runtime-dependency inspection;
- capability self-test output;
- Probe and Decode results for each legally shareable fixture;
- RGB channel count, 16-bit output and orientation/display bounds;
- Chinese/Unicode path result;
- CLI diagnostics result;
- GUI startup/pack/restore result;
- codesign, notarization and DMG results, or an explicit NOT RUN.

## Fixture policy

Use a local fixture directory such as:

~~~text
/Users/example/PhotoSheet-fixtures/dng/
~~~

Do not commit private camera files or customer material. A fixture may be attached to a public test only when its license and redistribution permission are documented. Without a fixture, mark the corresponding integration test NOT RUN; a skipped test is not a decode PASS.

## Current public boundary

The default core build uses the pure-Go fallback. The native build requires the photosheet_native tag, cgo, a matching architecture artifact and the Adobe DNG SDK license review described in THIRD_PARTY_NOTICES.md. The public release workflow does not build or publish the native path.

For implementation details, use RAW_BACKEND.md and RAW_PIPELINE.md. Keep this document focused on handoff evidence rather than personal machine history.
