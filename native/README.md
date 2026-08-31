# Optional native RAW backend

The source tree contains a narrow native boundary for LibRaw, Adobe DNG SDK, JPEG XL and related codecs. It is deliberately not part of the default public core build.

## Public build policy

The following files may exist in a maintainer's local checkout, but are ignored by Git:

~~~text
internal/raw/native/windows/libraw.dll
internal/raw/native/darwin/arm64/libphotosheet_raw.dylib
internal/raw/native/darwin/amd64/libphotosheet_raw.dylib
~~~

They must not be uploaded to GitHub Releases or committed to the repository until the exact source archives, SDK terms, notices, source-offer obligations and binary hashes have been reviewed. The Adobe DNG SDK review is currently open and is recorded as NEEDS MANUAL LICENSE REVIEW in THIRD_PARTY_NOTICES.md.

## Enabling a reviewed local build

After a maintainer has completed that review and built the corresponding artifact:

~~~text
go test -tags photosheet_native ./internal/raw
go build -tags photosheet_native ./cmd/photosheet
~~~

The Windows backend is used only on Windows amd64. The Darwin loader supports arm64 and amd64 with cgo. The ordinary core build leaves the native backend unavailable and uses the pure-Go DNG fallback.

## Rebuilding

The macOS helper is scripts/build-native-macos.sh. It verifies the LibRaw archive hash and can use a local Adobe DNG SDK archive supplied through PHOTOSHEET_NATIVE_DNG_SDK_ARCHIVE. It does not turn the SDK into a public dependency or bypass its license terms.

The Windows DLL must be built with a consistent MSVC runtime and the reviewed LibRaw/DNG SDK configuration. No Windows SDK or Adobe SDK source is vendored here. Record the toolchain, source hashes, compiler definitions, exported ABI and artifact hash in native/third_party.lock.json after review.

Do not use a Homebrew or developer-machine absolute runtime path in a release artifact. Do not use an embedded preview as final RAW pixels. Native output must be validated at the Go boundary as a non-empty 16-bit, three-channel RGB buffer.
