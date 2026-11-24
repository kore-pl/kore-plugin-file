//go:build linux && (amd64 || arm64)

package fileop

import (
	"io/fs"
	"syscall"
	"unsafe"
)

const (
	// renameNoReplaceFlag 是 renameat2 的 RENAME_NOREPLACE：
	// 目标已存在时直接返回 EEXIST，绝不替换（Linux 3.15+）。
	renameNoReplaceFlag = 1
	// atFDCWD 是 AT_FDCWD = -100，按内核 ABI 以 uintptr 传递。
	atFDCWD = ^uintptr(99)
)

// renameNoReplace 用 renameat2(RENAME_NOREPLACE) 把「检查目标不存在 + 改名」
// 压进**一次**系统调用：内核保证并发抢同一目标时恰好一个成功，其余返回 EEXIST。
// 因此多进程 / 多实例（site 插件的真实场景）也不会静默覆盖。
func renameNoReplace(src, dst string) error {
	srcPtr, err := syscall.BytePtrFromString(src)
	if err != nil {
		return &fs.PathError{Op: "renameat2", Path: src, Err: err}
	}
	dstPtr, err := syscall.BytePtrFromString(dst)
	if err != nil {
		return &fs.PathError{Op: "renameat2", Path: dst, Err: err}
	}
	_, _, errno := syscall.Syscall6(sysRenameat2,
		atFDCWD, uintptr(unsafe.Pointer(srcPtr)),
		atFDCWD, uintptr(unsafe.Pointer(dstPtr)),
		renameNoReplaceFlag, 0)
	switch errno {
	case 0:
		return nil
	case syscall.EEXIST, syscall.ENOTEMPTY:
		// 目标已存在（目录非空时内核报 ENOTEMPTY）：交给上层映射 E_CONFLICT。
		return fs.ErrExist
	case syscall.ENOSYS, syscall.EINVAL:
		// 内核 < 3.15，或该文件系统不认 RENAME_NOREPLACE（部分网络/旧 overlay fs）：
		// 退化为「检查后改名」，窗口见 rename_noreplace.go 的注释。
		return renameNoReplaceChecked(src, dst)
	}
	return &fs.PathError{Op: "renameat2", Path: src, Err: errno}
}
