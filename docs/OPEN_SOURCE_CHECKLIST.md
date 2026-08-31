# Open-source readiness checklist

This checklist records the current state of the repository before a public GitHub launch.

## Completed in this pass

- [x] Remove generated build outputs, dependency directories, and OS metadata from the candidate tree.
- [x] Ignore generated outputs, local native binaries, IDE files, caches, and credentials.
- [x] Scrub private filesystem paths from tracked documentation.
- [x] Run targeted secret-pattern and credential-filename scans.
- [x] Add English and Simplified Chinese project documentation.
- [x] Document architecture, supported formats, RAW boundaries, build modes, and release scope.
- [x] Add contribution, security, conduct, changelog, roadmap, issue templates, and Dependabot configuration.
- [x] Add CI, CodeQL, and tag-based release workflow definitions.
- [x] Make the public core build independent of local native RAW binaries.
- [x] Preserve the native RAW implementation as an explicitly optional, separately reviewed path.
- [x] Select and add the Apache-2.0 root LICENSE file.
- [x] Run a fresh-clone build.

## Still required before native builds or expanded releases

- [ ] Manually review the exact Adobe DNG SDK archive and redistribution terms.
- [ ] Reconcile native dependency notices and the local lock file with the exact shipped artifacts.
- [ ] Decide whether native RAW binaries may be distributed, and publish the required notices if yes.
- [ ] Validate macOS and Windows native builds on their target platforms.
- [ ] Validate installer, signing, notarization, and update-channel behavior.

## Current gate

Status: **READY — source-only core repository**.

The core source tree may be published under Apache-2.0. Native RAW binaries and expanded native/installer releases remain blocked until their legal and platform review is complete.
