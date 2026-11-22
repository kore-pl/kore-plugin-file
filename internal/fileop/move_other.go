//go:build !unix

package fileop

// isCrossDevice 在非 Unix 平台无法可靠判定跨卷错误，不启用复制回退。
func isCrossDevice(error) bool { return false }
