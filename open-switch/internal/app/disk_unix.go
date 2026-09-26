//go:build !windows

// 本文件负责Unix 平台磁盘空间查询。

package app

import "syscall"

func diskSpace(path string) (free, total uint64, err error) {
	var stat syscall.Statfs_t
	if err = syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), stat.Blocks * uint64(stat.Bsize), nil
}
