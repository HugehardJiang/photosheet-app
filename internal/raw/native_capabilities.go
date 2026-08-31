package raw

// Capability values are part of the native bridge ABI. Keep the numeric
// assignments stable across Windows and Darwin artifacts.
const (
	nativeCapDNGSDK uint32 = 1 << iota
	nativeCapJPEGXL
	nativeCap16Bit
	nativeCapDNGOpcode
	nativeCapSRGB
	nativeCapUnicodePaths
)

const nativeRequiredCapabilities = nativeCapDNGSDK |
	nativeCapJPEGXL |
	nativeCap16Bit |
	nativeCapDNGOpcode |
	nativeCapSRGB |
	nativeCapUnicodePaths
