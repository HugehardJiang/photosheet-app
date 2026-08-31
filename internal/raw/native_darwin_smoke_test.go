//go:build darwin && (amd64 || arm64)

package raw

import (
	"os"
	"strings"
	"testing"
)

func TestDarwinNativeBackendCapabilities(t *testing.T) {
	if strings.TrimSpace(os.Getenv("PHOTOSHEET_NATIVE_RAW_LIBRARY")) == "" && len(nativeDarwinEmbedded) == 0 {
		t.Skip("macOS native RAW artifact is not configured")
	}

	backend, err := newNativeBackend()
	if err != nil {
		t.Fatal(err)
	}
	capabilities := backend.Capabilities()
	if !capabilities.DNGSDKEnabled || !capabilities.SupportsJPEGXL ||
		!capabilities.Supports16BitRGB || !capabilities.DNGOpcodeProcessing ||
		!capabilities.SupportsUnicodePaths || capabilities.OutputColorSpace != "sRGB" {
		t.Fatalf("unexpected Darwin native capabilities: %#v", capabilities)
	}
}
