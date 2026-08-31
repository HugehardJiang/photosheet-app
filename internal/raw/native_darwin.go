//go:build darwin && (amd64 || arm64) && cgo

package raw

/*
#cgo darwin LDFLAGS: -ldl
#include <dlfcn.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

// dlsym returns a POSIX symbol address. The conversion to a callable function
// pointer deliberately stays in this C shim; Go must not jump through a
// uintptr returned by dlsym.
typedef int (*photosheet_capabilities_fn)(uint32_t *, char *, uint64_t);
typedef struct photosheet_raw_info {
    uint32_t width;
    uint32_t height;
    uint32_t channels;
    uint32_t bits;
    uint32_t orientation;
} photosheet_raw_info;
typedef int (*photosheet_probe_utf8_fn)(const char *, photosheet_raw_info *, char *, uint64_t);
typedef int (*photosheet_decode_utf8_fn)(const char *, unsigned char **, uint64_t *, photosheet_raw_info *, char *, uint64_t);
typedef void (*photosheet_free_fn)(void *);

static void *photosheet_dlopen(const char *path) {
    return dlopen(path, RTLD_NOW | RTLD_LOCAL);
}

static void *photosheet_dlsym(void *handle, const char *name) {
    (void)dlerror();
    return dlsym(handle, name);
}

static const char *photosheet_dlerror_message(void) {
    const char *message = dlerror();
    return message ? message : "dynamic loader error";
}

static int photosheet_call_capabilities(void *fn, uint32_t *flags, char *error, uint64_t error_capacity) {
    return ((photosheet_capabilities_fn)fn)(flags, error, error_capacity);
}

static int photosheet_call_probe_utf8(void *fn, const char *path,
                                      uint32_t *width, uint32_t *height,
                                      uint32_t *channels, uint32_t *bits,
                                      uint32_t *orientation, char *error,
                                      uint64_t error_capacity) {
    photosheet_raw_info info = {0, 0, 0, 0, 0};
    int result = ((photosheet_probe_utf8_fn)fn)(path, &info, error, error_capacity);
    if (result == 0) {
        *width = info.width;
        *height = info.height;
        *channels = info.channels;
        *bits = info.bits;
        *orientation = info.orientation;
    }
    return result;
}

static int photosheet_call_decode_utf8(void *fn, const char *path,
                                       unsigned char **data, uint64_t *data_size,
                                       uint32_t *width, uint32_t *height,
                                       uint32_t *channels, uint32_t *bits,
                                       uint32_t *orientation, char *error,
                                       uint64_t error_capacity) {
    photosheet_raw_info info = {0, 0, 0, 0, 0};
    int result = ((photosheet_decode_utf8_fn)fn)(path, data, data_size, &info,
                                                 error, error_capacity);
    if (result == 0) {
        *width = info.width;
        *height = info.height;
        *channels = info.channels;
        *bits = info.bits;
        *orientation = info.orientation;
    }
    return result;
}

static void photosheet_call_free(void *fn, void *data) {
    if (fn && data) {
        ((photosheet_free_fn)fn)(data);
    }
}

static void photosheet_copy_bytes(void *destination, const void *source, size_t size) {
    memcpy(destination, source, size);
}
*/
import "C"

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"
)

type nativeLibrary struct {
	handle       unsafe.Pointer
	probe        unsafe.Pointer
	decode       unsafe.Pointer
	free         unsafe.Pointer
	capabilities unsafe.Pointer
	flags        uint32
	path         string
}

var nativeLibraryState struct {
	once sync.Once
	lib  *nativeLibrary
	err  error
}

func newNativeBackend() (RawBackend, error) {
	if _, err := loadNativeLibrary(); err != nil {
		return nil, err
	}
	return nativeBackend{capabilities: nativeRuntimeBackendCapabilities()}, nil
}

func loadNativeLibrary() (*nativeLibrary, error) {
	nativeLibraryState.once.Do(func() {
		path, err := nativeDarwinLibraryPath()
		if err != nil {
			nativeLibraryState.err = err
			return
		}
		cpath := C.CString(path)
		defer C.free(unsafe.Pointer(cpath))
		handle := C.photosheet_dlopen(cpath)
		if handle == nil {
			nativeLibraryState.err = fmt.Errorf("dlopen %s failed: %s", path, C.GoString(C.photosheet_dlerror_message()))
			return
		}
		lookup := func(name string) unsafe.Pointer {
			csymbol := C.CString(name)
			defer C.free(unsafe.Pointer(csymbol))
			return C.photosheet_dlsym(handle, csymbol)
		}
		probe := lookup("photosheet_raw_probe_utf8")
		decode := lookup("photosheet_raw_decode_utf8")
		free := lookup("photosheet_raw_free")
		capabilities := lookup("photosheet_raw_capabilities")
		if probe == nil || decode == nil || free == nil || capabilities == nil {
			nativeLibraryState.err = fmt.Errorf("Darwin native RAW ABI is incomplete in %s", path)
			return
		}

		var flags C.uint32_t
		errorBuffer := make([]byte, 1024)
		code := C.photosheet_call_capabilities(
			capabilities,
			(*C.uint32_t)(unsafe.Pointer(&flags)),
			(*C.char)(unsafe.Pointer(&errorBuffer[0])),
			C.uint64_t(len(errorBuffer)),
		)
		if code != 0 {
			nativeLibraryState.err = nativeCallError(int32(code), errorBuffer)
			return
		}
		if uint32(flags)&nativeRequiredCapabilities != nativeRequiredCapabilities {
			nativeLibraryState.err = fmt.Errorf("native RAW capability self-test failed: flags=0x%08x required=0x%08x", uint32(flags), nativeRequiredCapabilities)
			return
		}
		nativeLibraryState.lib = &nativeLibrary{
			handle: handle, probe: probe, decode: decode, free: free,
			capabilities: capabilities, flags: uint32(flags), path: path,
		}
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

func nativeDarwinLibraryPath() (string, error) {
	if len(nativeDarwinEmbedded) > 0 {
		return materializeDarwinLibrary(nativeDarwinEmbedded)
	}

	// This override is intentionally opt-in and requires an absolute path. It
	// is useful for local bridge development; release builds should use the
	// embedded artifact or the App bundle Frameworks directory.
	if override := strings.TrimSpace(os.Getenv("PHOTOSHEET_NATIVE_RAW_LIBRARY")); override != "" {
		if !filepath.IsAbs(override) {
			return "", errors.New("PHOTOSHEET_NATIVE_RAW_LIBRARY must be an absolute path")
		}
		if err := requireRegularFile(override); err != nil {
			return "", fmt.Errorf("native RAW override %q: %w", override, err)
		}
		return override, nil
	}

	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate PhotoSheet executable: %w", err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(executable); resolveErr == nil {
		executable = resolved
	}
	executableDir := filepath.Dir(executable)
	candidates := []string{
		filepath.Join(executableDir, "..", "Frameworks", "libphotosheet_raw.dylib"),
		filepath.Join(executableDir, "libphotosheet_raw.dylib"),
	}
	for _, candidate := range candidates {
		if err := requireRegularFile(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("macOS native RAW artifact is unavailable for %s; build with -tags photosheet_native or bundle libphotosheet_raw.dylib", runtime.GOARCH)
}

func requireRegularFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file")
	}
	return nil
}

func materializeDarwinLibrary(data []byte) (string, error) {
	digest := sha256.Sum256(data)
	version := hex.EncodeToString(digest[:8])
	directory := filepath.Join(os.TempDir(), "PhotoSheet", "raw-backend", "darwin-"+runtime.GOARCH)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", fmt.Errorf("create private native RAW directory: %w", err)
	}
	path := filepath.Join(directory, "libphotosheet_raw-"+version+".dylib")
	if current, err := os.ReadFile(path); err == nil && len(current) == len(data) && sha256.Sum256(current) == digest {
		if err := os.Chmod(path, 0o700); err != nil {
			return "", fmt.Errorf("protect native RAW artifact: %w", err)
		}
		return path, nil
	}

	temporary, err := os.CreateTemp(directory, ".libphotosheet_raw-*")
	if err != nil {
		return "", fmt.Errorf("create native RAW staging file: %w", err)
	}
	temporaryPath := temporary.Name()
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o700); err == nil {
		_, err = temporary.Write(data)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", fmt.Errorf("write native RAW artifact: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		if existing, readErr := os.ReadFile(path); readErr == nil && len(existing) == len(data) && sha256.Sum256(existing) == digest {
			return path, nil
		}
		return "", fmt.Errorf("install native RAW artifact: %w", err)
	}
	keep = true
	return path, nil
}

func nativeProbe(filename string) (nativeProbeInfo, error) {
	lib, err := loadNativeLibrary()
	if err != nil {
		return nativeProbeInfo{}, fmt.Errorf("%w: %v", ErrBackendUnavailable, err)
	}
	if strings.IndexByte(filename, 0) >= 0 {
		return nativeProbeInfo{}, errors.New("native RAW path contains NUL byte")
	}
	path := C.CString(filename)
	defer C.free(unsafe.Pointer(path))
	var width, height, channels, bits, orientation C.uint32_t
	errorBuffer := make([]byte, 1024)
	code := C.photosheet_call_probe_utf8(
		lib.probe, path,
		(*C.uint32_t)(unsafe.Pointer(&width)),
		(*C.uint32_t)(unsafe.Pointer(&height)),
		(*C.uint32_t)(unsafe.Pointer(&channels)),
		(*C.uint32_t)(unsafe.Pointer(&bits)),
		(*C.uint32_t)(unsafe.Pointer(&orientation)),
		(*C.char)(unsafe.Pointer(&errorBuffer[0])),
		C.uint64_t(len(errorBuffer)),
	)
	if code != 0 {
		return nativeProbeInfo{}, nativeCallError(int32(code), errorBuffer)
	}
	return nativeProbeInfo{width: uint32(width), height: uint32(height), channels: uint32(channels), bits: uint32(bits), orientation: uint32(orientation)}, nil
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
	if strings.IndexByte(filename, 0) >= 0 {
		return nativeDecodeResult{}, errors.New("native RAW path contains NUL byte")
	}
	path := C.CString(filename)
	defer C.free(unsafe.Pointer(path))
	var data *C.uchar
	var dataSize C.uint64_t
	var width, height, channels, bits, orientation C.uint32_t
	errorBuffer := make([]byte, 1024)
	code := C.photosheet_call_decode_utf8(
		lib.decode, path,
		(**C.uchar)(unsafe.Pointer(&data)),
		(*C.uint64_t)(unsafe.Pointer(&dataSize)),
		(*C.uint32_t)(unsafe.Pointer(&width)),
		(*C.uint32_t)(unsafe.Pointer(&height)),
		(*C.uint32_t)(unsafe.Pointer(&channels)),
		(*C.uint32_t)(unsafe.Pointer(&bits)),
		(*C.uint32_t)(unsafe.Pointer(&orientation)),
		(*C.char)(unsafe.Pointer(&errorBuffer[0])),
		C.uint64_t(len(errorBuffer)),
	)
	if code != 0 {
		return nativeDecodeResult{}, nativeCallError(int32(code), errorBuffer)
	}
	if data == nil || dataSize == 0 || uint64(dataSize) > uint64(^uint(0)>>1) {
		if data != nil {
			C.photosheet_call_free(lib.free, unsafe.Pointer(data))
		}
		return nativeDecodeResult{}, errors.New("native backend returned an invalid image buffer")
	}
	defer C.photosheet_call_free(lib.free, unsafe.Pointer(data))
	if err := ctx.Err(); err != nil {
		return nativeDecodeResult{}, err
	}
	copyData := make([]byte, int(dataSize))
	C.photosheet_copy_bytes(unsafe.Pointer(&copyData[0]), unsafe.Pointer(data), C.size_t(dataSize))
	return nativeDecodeResult{
		data: copyData,
		info: nativeProbeInfo{width: uint32(width), height: uint32(height), channels: uint32(channels), bits: uint32(bits), orientation: uint32(orientation)},
	}, nil
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
