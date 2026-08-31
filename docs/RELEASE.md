# Release process

The public release workflow is tag-driven:

~~~text
vMAJOR.MINOR.PATCH
~~~

Pushing a matching tag builds the core CLI and updater for Linux x64/arm64, macOS Intel/Apple Silicon and Windows x64/arm64 with CGO disabled and the public pure-Go RAW fallback. It uploads the binaries and a SHA256SUMS file to a GitHub Release.

## What is not automated yet

Native RAW binaries, GUI installers, code signing, macOS notarization, Windows signing certificates and Linux desktop packaging are intentionally excluded from the current stable workflow. They require platform validation and a completed third-party license review. A missing signing secret must never prevent ordinary CI or an unsigned core build.

If native artifacts are later approved, add them only through a separate reviewed workflow and include the exact notices, source offer and hashes. Never put Apple IDs, notarization passwords, signing certificates, certificate passwords or updater signing keys in the repository. Use GitHub Actions Secrets for release-only credentials.

## Manual pre-release checklist

- Confirm the project license has been selected and committed.
- Run the core test, vet and frontend checks from README.md.
- Validate the tag matches the intended semantic version.
- Review the generated file list for secrets, private paths, media and large files.
- Rebuild or verify every native artifact from the reviewed lock entry.
- Generate SHA256SUMS over release artifacts only.
- Verify the release notes do not claim native or installer support that was not built.

Updater manifests are signed separately with an Ed25519 key. The private signing key is deployment infrastructure and must never be committed.
