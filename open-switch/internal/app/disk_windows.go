//go:build windows

package app

import "golang.org/x/sys/windows"

func diskSpace(path string) (free, total uint64, err error) {
	root, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}
	var available uint64
	err = windows.GetDiskFreeSpaceEx(root, &available, &total, &free)
	return free, total, err
}
