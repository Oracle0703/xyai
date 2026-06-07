//go:build windows

package diskspace

import "golang.org/x/sys/windows"

func usage(path string) (uint64, uint64, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}
	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	if err := windows.GetDiskFreeSpaceEx(p, &freeBytesAvailable, &totalBytes, &totalFreeBytes); err != nil {
		return 0, 0, err
	}
	return totalBytes, freeBytesAvailable, nil
}
