//go:build unix

package fileop

import (
	"errors"
	"syscall"
)

// isCrossDevice 判定 rename 是否因跨文件系统失败（EXDEV），据此启用复制回退。
func isCrossDevice(err error) bool { return errors.Is(err, syscall.EXDEV) }
