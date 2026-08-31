# RAW backend and legal boundary

PhotoSheet has two RAW layers:

1. photosheet-dng is a pure-Go fallback that probes TIFF/DNG metadata and decodes the tested uncompressed/Deflate subset.
2. libraw-dngsdk is an optional native backend behind the photosheet_native build tag.

The native backend is not needed for the public core build. Its local artifacts are ignored by Git:

~~~text
internal/raw/native/windows/libraw.dll
internal/raw/native/darwin/arm64/libphotosheet_raw.dylib
internal/raw/native/darwin/amd64/libphotosheet_raw.dylib
~~~

## Why it is opt-in

The local native build links or embeds components whose exact redistribution terms must travel with the artifact. The Adobe DNG SDK package and its bundled notices are not present in this repository, and the exact SDK build metadata in the development checkout has not been reconciled with the current official download page. This is a release blocker, not a build failure to hide.

Do not commit the SDK, sample DNGs, DLL/dylibs or generated native source. Do not enable the native release path until THIRD_PARTY_NOTICES.md is updated with the reviewed license texts and source obligations.

## Local Windows setup

Place a reviewed native DLL at internal/raw/native/windows/libraw.dll and build with:

~~~powershell
go test -tags photosheet_native ./internal/raw
go build -tags photosheet_native -trimpath -o photosheet.exe ./cmd/photosheet
~~~

Windows amd64 uses UTF-16 paths and extracts the embedded DLL to a private temporary directory. Windows ARM64 remains on the fallback path.

## Local macOS setup

Obtain the exact Adobe DNG SDK archive through Adobe's official DNG SDK page and review its accompanying terms. Set PHOTOSHEET_NATIVE_DNG_SDK_ARCHIVE to the local archive, verify its hash against the reviewed lock entry, and run:

~~~bash
scripts/build-native-macos.sh --arch arm64
scripts/build-native-macos.sh --arch amd64
go test -tags photosheet_native ./internal/raw -run TestDarwinNativeBackendCapabilities -count=1 -v
~~~

The loader uses a small C shim for dlopen/dlsym calls, materializes embedded bytes into a private temporary directory, checks capability flags, and validates the returned RGB16 buffer before copying it into Go memory. A release build must not retain Homebrew or developer-home absolute paths.

## Capability and fixture policy

The native self-test requires DNG SDK, JPEG XL, 16-bit output, DNG opcode processing, sRGB and Unicode path capabilities. Optional integration tests use PHOTOSHEET_NATIVE_DNG_FIXTURE or PHOTOSHEET_NATIVE_DNG_FIXTURES. Fixtures must be local, licensed for the test, and excluded from Git unless their redistribution rights are documented.

No current public release claims native JPEG/JPEG XL DNG support. The core matrix is documented in RAW_PIPELINE.md.
