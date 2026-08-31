//go:build windows && amd64 && photosheet_native

package raw

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The native DLL is embedded and extracted to a per-version temp directory at
// runtime. This keeps the delivered Windows build a single portable EXE while
// avoiding a dependency on the user's current working directory.
//
//go:embed native/windows/libraw.dll
var nativeDLL []byte

type nativeLibrary struct {
	dll          *windows.DLL
	probe        *windows.Proc
	decode       *windows.Proc
	free         *windows.Proc
	capabilities *windows.Proc
	flags        uint32
}

var nativeLibraryState struct {
	once sync.Once
	lib  *nativeLibrary
	err  error
}

var nativeMoveMemory = windows.NewLazySystemDLL("kernel32.dll").NewProc("RtlMoveMemory")

func newNativeBackend() (RawBackend, error) {
	if _, err := loadNativeLibrary(); err != nil {
		return nil, err
	}
	return nativeBackend{capabilities: nativeRuntimeBackendCapabilities()}, nil
}

func loadNativeLibrary() (*nativeLibrary, error) {
	nativeLibraryState.once.Do(func() {
		digest := sha256.Sum256(nativeDLL)
		version := hex.EncodeToString(digest[:8])
		directory := filepath.Join(os.TempDir(), "PhotoSheet", "raw-backend")
		if err := os.MkdirAll(directory, 0o700); err != nil {
			nativeLibraryState.err = err
			return
		}
		path := filepath.Join(directory, "libraw-"+version+".dll")
		if data, err := os.ReadFile(path); err != nil || len(data) != len(nativeDLL) || sha256.Sum256(data) != digest {
			if err := os.WriteFile(path, nativeDLL, 0o700); err != nil {
				nativeLibraryState.err = err
				return
			}
		}
		dll, err := windows.LoadDLL(path)
		if err != nil {
			nativeLibraryState.err = err
			return
		}
		probe, err := dll.FindProc("photosheet_raw_probe_w")
		if err != nil {
			nativeLibraryState.err = err
			return
		}
		decode, err := dll.FindProc("photosheet_raw_decode_w")
		if err != nil {
			nativeLibraryState.err = err
			return
		}
		free, err := dll.FindProc("photosheet_raw_free")
		if err != nil {
			nativeLibraryState.err = err
			return
		}
		capabilities, err := dll.FindProc("photosheet_raw_capabilities")
		if err != nil {
			nativeLibraryState.err = err
			return
		}
		var flags uint32
		errorBuffer := make([]byte, 1024)
		r1, _, _ := capabilities.Call(
			uintptr(unsafe.Pointer(&flags)),
			uintptr(unsafe.Pointer(&errorBuffer[0])),
			uintptr(len(errorBuffer)),
		)
		if code := int32(r1); code != 0 {
			nativeLibraryState.err = nativeCallError(code, errorBuffer)
			return
		}
		if flags&nativeRequiredCapabilities != nativeRequiredCapabilities {
			nativeLibraryState.err = fmt.Errorf("native RAW capability self-test failed: flags=0x%08x required=0x%08x", flags, nativeRequiredCapabilities)
			return
		}
		nativeLibraryState.lib = &nativeLibrary{dll: dll, probe: probe, decode: decode, free: free, capabilities: capabilities, flags: flags}
	})
	return nativeLibraryState.lib, nativeLibraryState.err
}

func nativeRuntimeBackendCapabilities() BackendCapabilities {
	capabilities := nativeBackend{}.Capabilities()
	lib, err := loadNativeLibrary()
	if err != nil || lib == nil {
		return capabilities
	}
	capabilities.DNGSDKEnabled = lib.flags&nativeCapDNGSDK != 0
	capabilities.SupportsJPEGXL = lib.flags&nativeCapJPEGXL != 0
	capabilities.Supports16BitRGB = lib.flags&nativeCap16Bit != 0
	capabilities.DNGOpcodeProcessing = lib.flags&nativeCapDNGOpcode != 0
	capabilities.SupportsUnicodePaths = lib.flags&nativeCapUnicodePaths != 0
	capabilities.OutputColorSpace = "sRGB"
	return capabilities
}

func nativeProbe(filename string) (nativeProbeInfo, error) {
	lib, err := loadNativeLibrary()
	if err != nil {
		return nativeProbeInfo{}, fmt.Errorf("%w: %v", ErrBackendUnavailable, err)
	}
	path, err := windows.UTF16PtrFromString(filename)
	if err != nil {
		return nativeProbeInfo{}, err
	}
	var info nativeProbeInfo
	errorBuffer := make([]byte, 1024)
	r1, _, _ := lib.probe.Call(
		uintptr(unsafe.Pointer(path)),
		uintptr(unsafe.Pointer(&info)),
		uintptr(unsafe.Pointer(&errorBuffer[0])),
		uintptr(len(errorBuffer)),
	)
	code := int32(r1)
	if code != 0 {
		return nativeProbeInfo{}, nativeCallError(code, errorBuffer)
	}
	return info, nil
}

type nativeDecodeResult struct {
	data []byte
	info nativeProbeInfo
}

func nativeDecode(ctx context.Context, filename string) (nativeDecodeResult, error) {
	if err := ctx.Err(); err != nil {
		return nativeDecodeResult{}, err
	}
	lib, err := loadNativeLibrary()
	if err != nil {
		return nativeDecodeResult{}, fmt.Errorf("%w: %v", ErrBackendUnavailable, err)
	}
	path, err := windows.UTF16PtrFromString(filename)
	if err != nil {
		return nativeDecodeResult{}, err
	}
	var data uintptr
	var dataSize uint64
	var info nativeProbeInfo
	errorBuffer := make([]byte, 1024)
	r1, _, _ := lib.decode.Call(
		uintptr(unsafe.Pointer(path)),
		uintptr(unsafe.Pointer(&data)),
		uintptr(unsafe.Pointer(&dataSize)),
		uintptr(unsafe.Pointer(&info)),
		uintptr(unsafe.Pointer(&errorBuffer[0])),
		uintptr(len(errorBuffer)),
	)
	code := int32(r1)
	if code != 0 {
		return nativeDecodeResult{}, nativeCallError(code, errorBuffer)
	}
	if data == 0 || dataSize == 0 || dataSize > uint64(^uint(0)>>1) {
		return nativeDecodeResult{}, errors.New("native backend returned an invalid image buffer")
	}
	defer lib.free.Call(data)
	if err := ctx.Err(); err != nil {
		return nativeDecodeResult{}, err
	}
	copyData := make([]byte, int(dataSize))
	if len(copyData) > 0 {
		// Copy through a Windows memory primitive. This avoids converting a
		// native uintptr into a Go slice and keeps the allocation domain clear.
		_, _, _ = nativeMoveMemory.Call(uintptr(unsafe.Pointer(&copyData[0])), data, uintptr(len(copyData)))
	}
	return nativeDecodeResult{data: copyData, info: info}, nil
}

func nativeCallError(code int32, buffer []byte) error {
	message := string(buffer)
	for i, char := range message {
		if char == 0 {
			message = message[:i]
			break
		}
	}
	if message == "" {
		message = "native backend error"
	}
	kind := ErrRawDecode
	switch code {
	case -2, -6, -8:
		kind = ErrRawUnsupportedVariant
	case -100007, -100013, -100012:
		kind = ErrRawOutOfMemory
	case -100008:
		kind = ErrRawCorrupt
	case -100010:
		kind = ErrRawCancelled
	case -100009:
		kind = ErrRawDecode
	}
	return &BackendError{Code: code, Kind: kind, Message: message}
}
