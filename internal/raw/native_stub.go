//go:build !darwin && (!windows || !amd64)

package raw

import "context"

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
