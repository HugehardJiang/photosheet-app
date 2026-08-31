# project.json format

The project file is UTF-8 JSON. The current on-disk schema is version 3; older versions are migrated in memory and are written as version 3 only when the user explicitly saves or upgrades the project.

## Coordinate system

- Origin: top-left.
- X increases to the right.
- Y increases downward.
- All coordinates are integer pixels.
- Rectangles use half-open bounds: [x, x + width) × [y, y + height).

Thus a rectangle with x=10, width=20 covers columns 10 through 29. contentRect is always the crop rectangle; outerRect includes padding.

## Top-level object

~~~json
{
  "schemaVersion": 3,
  "appVersion": "0.1.0",
  "projectId": "opaque-stable-id",
  "createdAt": "2026-01-01T00:00:00Z",
  "mode": "strict-lossless",
  "settings": {},
  "sheets": []
}
~~~

mode is strict-lossless or jpeg-compatibility. A JPEG sheet is valid only in JPEG Compatibility mode. Unknown fields should be ignored when reading.

## Settings

Important fields are:

| Field | Meaning |
| --- | --- |
| maximumLongEdge, maximumShortEdge, maximumTotalPixels | Sheet size constraints. At least one constraint is required. |
| format | png, tiff, or jpg; jpg requires JPEG Compatibility mode. |
| padding, paddingMode | Outer spacing around each image. |
| allowRotation | Allows a 90° packing rotation. |
| workingBitDepth | 8 or 16; new projects default to 16. |
| workingColorSpace | Currently operationally supported as sRGB; other values are validated but not implemented by the fallback RAW path. |
| tiffCompression | zip, lzw, or none, subject to encoder support. |
| storageMode | low-disk, balanced, or performance for tile compression. |
| rawProcessing | White balance, demosaic and optional processing flags for RAW decode. |

## Sheet

~~~json
{
  "id": "01234567-S001",
  "index": 1,
  "canonicalStem": "PS_01234567_S001",
  "file": "PS_01234567_S001.png",
  "width": 16384,
  "height": 12000,
  "format": "png",
  "iccProfileHash": "optional-sha256",
  "images": []
}
~~~

id, index and canonicalStem are the stable resolver identity. file is the generated sheet filename and must be a safe relative filename; it cannot contain path traversal.

## Image

Each images[] entry contains:

~~~json
{
  "id": "image-000001",
  "filename": "DSC0001.jpg",
  "relativePath": "DSC0001.jpg",
  "sha256": "source-file-sha256",
  "fileSize": 1234567,
  "encodedWidth": 6000,
  "encodedHeight": 4000,
  "displayWidth": 6000,
  "displayHeight": 4000,
  "exifOrientation": 1,
  "rotation": 0,
  "padding": 16,
  "sourceFormat": "jpg",
  "sourceBitDepth": 8,
  "outerRect": { "x": 0, "y": 0, "width": 6032, "height": 4032 },
  "contentRect": { "x": 16, "y": 16, "width": 6000, "height": 4000 }
}
~~~

encodedWidth/encodedHeight describe the file encoding. displayWidth/displayHeight describe the pixels after EXIF orientation is normalized. rotation is the additional 90° packing rotation, not the camera orientation. outerRect contains contentRect; extraction uses only contentRect and reverses rotation.

RAW images may add raw metadata, including DNG version, compression tag/state, CFA, black/white levels, backend, decode availability and effective bit depth. rawSourceIntegrity describes the source compression/RAW boundary; it must not be read as a promise that the demosaiced RGB output is sensor-identical.

## Resolver behavior

Processed sheets may be in another directory. The resolver accepts explicitly selected files/directories and optional manual bindings. It recognizes exact stems and clearly delimited processed suffixes, but it does not select a candidate merely because a token appears in its name. Multiple safe candidates produce ambiguous.

Current statuses include matched, ambiguous, missing, invalid, uniform-scaled and unsafe-transform. Uniform scaling requires explicit user confirmation; non-uniform scaling is rejected.
