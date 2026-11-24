//go:build linux && arm64

package fileop

// sysRenameat2 是 linux/arm64 的 renameat2 系统调用号（内核 3.15+）。
// 见 renameat2_linux_amd64.go 的说明：不引入 golang.org/x/sys，按架构显式声明。
const sysRenameat2 uintptr = 276
