package fileop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// TestTrashReservedPath 回收站路径不能经普通方法访问，且根目录列表不展示它。
func TestTrashReservedPath(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "a.txt", "x")
	mustWrite(t, root, "keep.txt", "x")
	var deleted DeleteResult
	if perr := mustCall(t, "file.delete", `{"path":"a.txt"}`, root, &deleted); perr != nil {
		t.Fatal(perr)
	}
	for _, c := range []struct{ method, params string }{
		{"file.list", `{"path":".trash"}`},
		{"file.stat", `{"path":".trash"}`},
		{"file.read", `{"path":".trash/entries"}`},
		{"file.write", `{"path":".trash/x","content":"y"}`},
		{"file.delete", `{"path":".trash","recursive":true}`},
		{"file.mkdir", `{"path":".trash/d"}`},
		{"file.copy", `{"src":"keep.txt","dst":".trash/keep.txt"}`},
		{"file.compress", `{"paths":["."],"archive_path":".trash/x.zip"}`},
	} {
		_, perr := Call(c.method, json.RawMessage(c.params), root, nil)
		if perr == nil || perr.Code != pluginapi.E_INVALID {
			t.Fatalf("%s %s 错误=%v，期望 E_INVALID", c.method, c.params, perr)
		}
	}
	var listing ListResult
	if perr := mustCall(t, "file.list", `{"path":"."}`, root, &listing); perr != nil {
		t.Fatalf("列根目录失败: %v", perr)
	}
	for _, entry := range listing.Entries {
		if entry.Name == TrashDirName {
			t.Fatalf("根目录列表不应展示回收站: %+v", listing.Entries)
		}
	}
}

// TestTrashBaseRejectsSymlink 回收站目录被替换成指向根外的符号链接时，删除必须拒绝。
func TestTrashBaseRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, TrashDirName)); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, root, "a.txt", "x")
	if _, perr := Call("file.delete", json.RawMessage(`{"path":"a.txt"}`), root, nil); perr == nil || perr.Code != pluginapi.E_INVALID {
		t.Fatalf("回收站为逃逸符号链接时删除错误=%v，期望 E_INVALID", perr)
	}
	if _, err := os.Stat(filepath.Join(outside, "a.txt")); !os.IsNotExist(err) {
		t.Fatalf("文件被搬出配置根目录: %v", err)
	}
}
