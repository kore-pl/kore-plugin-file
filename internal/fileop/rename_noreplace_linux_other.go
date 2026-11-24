//go:build linux && !amd64 && !arm64

package fileop

// renameNoReplace 在非 Tier A 的 Linux 架构上退化为「检查后改名」。
// G-16 的原子路径只承诺 linux/amd64 与 linux/arm64；其余架构保留功能但存在
// TOCTOU 窗口（窗口说明见 rename_noreplace.go）。
func renameNoReplace(src, dst string) error { return renameNoReplaceChecked(src, dst) }
