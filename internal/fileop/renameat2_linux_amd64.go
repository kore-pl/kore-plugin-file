//go:build linux && amd64

package fileop

// sysRenameat2 是 linux/amd64 的 renameat2 系统调用号（内核 3.15+）。
// Go 标准库 syscall 在 amd64 上未导出该常量（只在内核仅提供 renameat2 的
// loong64/riscv64 有），故按各架构 syscall 表显式声明，避免引入 golang.org/x/sys
// 依赖（插件仓只依赖 kore/pkg/pluginapi）。
const sysRenameat2 uintptr = 316
