//go:build windows && amd64 && !photosheet_native

package raw

import "context"

// The public core build deliberately excludes the locally supplied native
// backend. Enable the photosheet_native tag only after placing a legally
// redistributable LibRaw/DNG SDK artifact at the documented path.
func newNativeBackend() (RawBackend, error) {
	return nil, ErrRawBackendUnavailable
}

func nativeProbe(string) (nativeProbeInfo, error) {
	return nativeProbeInfo{}, ErrBackendUnavailable
}

type nativeDecodeResult struct {
	data []byte
	info nativeProbeInfo
}

func nativeDecode(context.Context, string) (nativeDecodeResult, error) {
	return nativeDecodeResult{}, ErrBackendUnavailable
}
