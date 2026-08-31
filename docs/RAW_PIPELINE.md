# RAW/DNG pipeline

PhotoSheet separates two guarantees:

- RAW source integrity: whether the source container/compression was read without an additional lossy step.
- RGB pixel integrity: whether the decoded working RGB pixels remain unchanged while they are packed into and extracted from a lossless sheet.

These are not the same guarantee. Demosaicing, white balance and conversion to a working color space necessarily turn sensor data into RGB data.

## Pipeline

~~~text
DNG / RAW
   ↓
metadata probe
   ↓
decode
   ↓
demosaic
   ↓
white balance
   ↓
working color space (currently sRGB)
   ↓
16-bit RGB
   ↓
PNG16 or lossless TIFF sheet
   ↓
contentRect crop and output
~~~

The scanner performs a low-cost probe and records DecodeAvailable plus DecodeReason. It must not hide a recognized but unsupported RAW file as “no image found”.

## Public core fallback

The pure-Go backend parses the TIFF/DNG IFD structure and supports the tested uncompressed and Deflate strip/full-width-tile subset. It can report metadata without demosaicing. It does not claim to decode JPEG-compressed DNG, lossy JPEG DNG, JPEG XL DNG or every camera-specific RAW variant.

## Optional native backend

The native boundary is behind the photosheet_native build tag. Development checkouts may contain local artifacts for Windows x64 or macOS arm64/x86_64, but those artifacts are ignored by Git and are not public release inputs while the Adobe DNG SDK redistribution review is open. CI and the public release workflow use the fallback build.

When a maintainer has a legally reviewed native build, it must:

- report its actual backend and capability flags;
- return a display-oriented 16-bit, three-channel RGB buffer;
- validate dimensions, channels and buffer size at the Go boundary;
- avoid using an embedded preview as final pixels;
- use a private runtime extraction directory for embedded libraries;
- keep all SDK notices and any required source offer with the release.

The expected local artifact paths are:

~~~text
internal/raw/native/windows/libraw.dll
internal/raw/native/darwin/arm64/libphotosheet_raw.dylib
internal/raw/native/darwin/amd64/libphotosheet_raw.dylib
~~~

The Windows and macOS build scripts support these local inputs, but they must not be used to imply that a clean public clone has native RAW support.

## DNG variants

| Variant | Probe | Core fallback decode | Native status |
| --- | --- | --- | --- |
| Uncompressed CFA/RGB | Yes | Supported subset | Optional |
| Deflate/ZIP | Yes | Supported subset | Optional |
| JPEG tag 7 | Yes | No | Requires reviewed native backend |
| Lossy JPEG tag 34892 | Yes | No | Requires reviewed native backend |
| JPEG XL tag 52546 | Yes | No | Requires reviewed native backend |

RAW metadata can include a camera model, DNG version, compression tag, CFA and bit depth. A sample DNG must never be added to the repository unless its redistribution rights are documented.
