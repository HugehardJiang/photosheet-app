# Third-party notices

This file records the direct dependencies used by the source tree and the native components referenced by the optional build. It is not a substitute for the complete license texts required by each dependency. The core repository does not vendor third-party source.

## Core application dependencies

| Dependency | Version | Purpose | License | Project URL | Redistribution notes |
| --- | --- | --- | --- | --- | --- |
| Wails | v2.10.2 | Native desktop shell and bindings | MIT | https://github.com/wailsapp/wails | Keep the Wails license notice in distributions. |
| Go image | v0.12.0 | Image codecs and image helpers | BSD-3-Clause | https://pkg.go.dev/golang.org/x/image | License text is in the upstream module. |
| Go sys | v0.30.0 | Platform system calls | BSD-3-Clause | https://pkg.go.dev/golang.org/x/sys | License text is in the upstream module. |
| React | 19.1.1 | Frontend UI | MIT | https://github.com/facebook/react | npm lockfile records the resolved package. |
| React DOM | 19.1.1 | Browser DOM renderer | MIT | https://github.com/facebook/react | npm lockfile records the resolved package. |
| TypeScript | 5.9.2 | Frontend type checking and compilation | Apache-2.0 | https://github.com/microsoft/TypeScript | Keep upstream notices when redistributing bundled tooling, if applicable. |
| Vite | 7.3.6 | Frontend build and development server | MIT | https://github.com/vitejs/vite | npm lockfile records the resolved package. |
| @vitejs/plugin-react | 5.0.2 | Vite React integration | MIT | https://github.com/vitejs/vite-plugin-react | npm lockfile records the resolved package. |
| @types/react | 19.1.10 | TypeScript declarations | MIT | https://github.com/DefinitelyTyped/DefinitelyTyped | Development-only package. |
| @types/react-dom | 19.1.7 | TypeScript declarations | MIT | https://github.com/DefinitelyTyped/DefinitelyTyped | Development-only package. |

Indirect Go and npm dependencies are tracked by go.sum and frontend/package-lock.json. A release that redistributes compiled frontend assets should generate a complete dependency notice bundle from the exact lockfiles.

## Optional native RAW dependencies

The following components are not part of the public core build and are not downloaded by CI:

| Dependency | Version | Purpose | License/status | Project URL | Redistribution notes |
| --- | --- | --- | --- | --- | --- |
| LibRaw | 0.22.2 | Native camera RAW/DNG decode | LGPL-2.1 or CDDL-1.0 according to the upstream project | https://www.libraw.org/ | A binary distribution must preserve the selected license obligations and provide any required corresponding source/build offer. |
| Adobe Digital Negative SDK | 1.7.1, build 2652 | DNG processing, including the native path used by the local backend | **NEEDS MANUAL LICENSE REVIEW**; the exact SDK package terms are not present in this repository | https://helpx.adobe.com/camera-raw/digital-negative.html | Do not commit the SDK, its source, its sample files or a linked binary until the exact package terms and notices have been reviewed. |
| libjxl | 0.11.2 | JPEG XL decode in the optional native path | BSD-3-Clause plus the upstream PATENTS grant | https://github.com/libjxl/libjxl | Preserve the license and patent notice for any binary distribution. |
| Highway | SDK-bundled version | SIMD support for libjxl | Review the exact bundled notice | https://github.com/google/highway | Exact version and notice must be recorded from the SDK package before native redistribution. |
| Brotli | SDK-bundled version | Compression dependency for libjxl | Review the exact bundled notice | https://github.com/google/brotli | Exact version and notice must be recorded from the SDK package before native redistribution. |
| skcms | SDK-bundled version | Color management dependency for libjxl | Review the exact bundled notice | https://skia.googlesource.com/skia/+/refs/heads/main/third_party/skcms/ | Exact version and notice must be recorded from the SDK package before native redistribution. |
| Adobe XMP Toolkit | SDK-bundled version | XMPCore metadata support in the native bridge | Review the exact bundled notice | https://github.com/adobe/XMP-Toolkit-SDK | The native bridge currently uses XMPCore only; preserve all applicable Adobe and bundled notices. |
| libjpeg | 9.3.0 in the recorded macOS build | JPEG support used by the native dependency build | Independent JPEG Group terms; verify the exact SDK copy | https://www.ijg.org/ | Preserve the upstream notice in any native distribution. |
| zlib | Platform/SDK build dependency | Compression support | zlib license | https://zlib.net/ | Preserve the upstream notice if shipped in a native artifact. |

The native lock manifest is native/third_party.lock.json. It records build inputs and hashes, not legal approval. The local DLL and dylibs are ignored by Git. Their presence in a developer checkout must not be interpreted as permission to publish them.

## Review gates

Before publishing any native artifact, the maintainer must:

1. retain the exact upstream license and notice files from the source archives;
2. determine whether the exact Adobe DNG SDK package permits the intended source and binary distribution;
3. document a corresponding-source offer if required by LibRaw or another component;
4. record the artifact-to-source build inputs and hashes;
5. update this file and the release workflow only after the legal review is complete.
