//go:build windows

package platform

import "golang.org/x/sys/windows"

func FreeBytes(path string) (uint64, error) {
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(windows.StringToUTF16Ptr(path), &free, &total, &totalFree); err != nil {
		return 0, err
	}
	return free, nil
}
