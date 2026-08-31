package platform

import "runtime"

func Name() string { return runtime.GOOS + "/" + runtime.GOARCH }
