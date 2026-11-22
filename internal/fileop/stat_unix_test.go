//go:build unix

package fileop

import (
	"os"
	"testing"
)

// TestStatOwnerUnix file.stat 的 owner 必须等于当前进程的 uid/gid。
func TestStatOwnerUnix(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "a.txt", "data")
	var got StatResult
	if perr := mustCall(t, "file.stat", `{"path":"a.txt"}`, root, &got); perr != nil {
		t.Fatalf("stat 失败: %v", perr)
	}
	if got.Owner.UID != uint32(os.Getuid()) || got.Owner.GID != uint32(os.Getgid()) {
		t.Fatalf("owner=%+v，期望 uid=%d gid=%d", got.Owner, os.Getuid(), os.Getgid())
	}
}
