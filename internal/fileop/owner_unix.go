//go:build unix

package fileop

import (
	"io/fs"
	"syscall"
)

// fileOwner 从 FileInfo 提取 uid/gid；非 unix 平台返回 0/0（见 owner_other.go）。
func fileOwner(info fs.FileInfo) (uint32, uint32) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0
	}
	return st.Uid, st.Gid
}
