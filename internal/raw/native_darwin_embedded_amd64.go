//go:build darwin && amd64 && cgo && photosheet_native

package raw

import _ "embed"

// The macOS amd64 build script places the verified native artifact here
// before compiling with -tags photosheet_native.
//
//go:embed native/darwin/amd64/libphotosheet_raw.dylib
var nativeDarwinEmbedded []byte
