//go:build !linux

package fileop

// renameNoReplace 在 macOS / Windows 上退化为「检查后改名」：这两个平台没有
// renameat2(RENAME_NOREPLACE) 的等价原语（Windows 需 MoveFileEx 语义定制，
// v1 preview 不承诺跨进程原子）。窗口说明见 rename_noreplace.go。
func renameNoReplace(src, dst string) error { return renameNoReplaceChecked(src, dst) }
