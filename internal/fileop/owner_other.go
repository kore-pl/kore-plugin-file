//go:build !unix

package fileop

import "io/fs"

// fileOwner 在非 unix 平台不提供 uid/gid（Tier B 降级），返回 0/0。
func fileOwner(fs.FileInfo) (uint32, uint32) { return 0, 0 }
