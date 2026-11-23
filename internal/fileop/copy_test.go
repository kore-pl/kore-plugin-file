package fileop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// TestCopyFile 单文件复制：内容与权限位保留、源仍在、统计正确。
func TestCopyFile(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "a.txt", "payload")
	if err := os.Chmod(filepath.Join(root, "a.txt"), 0o640); err != nil {
		t.Fatal(err)
	}
	var got CopyResult
	if perr := mustCall(t, "file.copy", `{"src":"a.txt","dst":"b.txt"}`, root, &got); perr != nil {
		t.Fatalf("复制失败: %v", perr)
	}
	if !got.Copied || got.Kind != "file" || got.Entries != 1 || got.Bytes != 7 || got.From != "a.txt" || got.To != "b.txt" {
		t.Fatalf("复制结果异常: %+v", got)
	}
	if data, err := os.ReadFile(filepath.Join(root, "b.txt")); err != nil || string(data) != "payload" {
		t.Fatalf("副本内容=%q err=%v", data, err)
	}
	if info, err := os.Stat(filepath.Join(root, "b.txt")); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("副本权限=%v err=%v，期望 0640", info.Mode().Perm(), err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "a.txt")); err != nil || string(data) != "payload" {
		t.Fatalf("源文件被改动: %q err=%v", data, err)
	}
}

// TestCopyDirRecursive 目录递归复制：嵌套文件与符号链接都保留（链接不跟随）。
func TestCopyDirRecursive(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, root, "src/sub/f.txt", "nested")
	if err := os.Symlink("sub/f.txt", filepath.Join(root, "src", "link")); err != nil {
		t.Fatal(err)
	}
	var got CopyResult
	if perr := mustCall(t, "file.copy", `{"src":"src","dst":"dst"}`, root, &got); perr != nil {
		t.Fatalf("复制目录失败: %v", perr)
	}
	if got.Kind != "dir" || got.Entries < 4 || got.Bytes != 6 {
		t.Fatalf("目录复制统计异常: %+v", got)
	}
	if data, err := os.ReadFile(filepath.Join(root, "dst", "sub", "f.txt")); err != nil || string(data) != "nested" {
		t.Fatalf("复制后内容=%q err=%v", data, err)
	}
	target, err := os.Readlink(filepath.Join(root, "dst", "link"))
	if err != nil || target != "sub/f.txt" {
		t.Fatalf("复制后链接=%q err=%v，期望 sub/f.txt", target, err)
	}
}

// TestCopyOverwritePolicy 覆盖策略显式：默认冲突；文件覆盖文件可；非空目录与类型不符都拒绝。
func TestCopyOverwritePolicy(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "a.txt", "new")
	mustWrite(t, root, "b.txt", "old")
	_, perr := Call("file.copy", json.RawMessage(`{"src":"a.txt","dst":"b.txt"}`), root, nil)
	if perr == nil || perr.Code != pluginapi.E_CONFLICT {
		t.Fatalf("冲突复制错误=%v，期望 E_CONFLICT", perr)
	}
	if perr := mustCall(t, "file.copy", `{"src":"a.txt","dst":"b.txt","overwrite":true}`, root, nil); perr != nil {
		t.Fatalf("覆盖复制失败: %v", perr)
	}
	if data, err := os.ReadFile(filepath.Join(root, "b.txt")); err != nil || string(data) != "new" {
		t.Fatalf("覆盖后内容=%q err=%v", data, "new")
	}
	if err := os.MkdirAll(filepath.Join(root, "full"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, root, "full/x.txt", "x")
	if err := os.MkdirAll(filepath.Join(root, "src-full"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, root, "src-full/y.txt", "y")
	_, perr = Call("file.copy", json.RawMessage(`{"src":"src-full","dst":"full","overwrite":true}`), root, nil)
	if perr == nil || perr.Code != pluginapi.E_CONFLICT {
		t.Fatalf("非空目录覆盖错误=%v，期望 E_CONFLICT", perr)
	}
	_, perr = Call("file.copy", json.RawMessage(`{"src":"a.txt","dst":"full","overwrite":true}`), root, nil)
	if perr == nil || perr.Code != pluginapi.E_CONFLICT {
		t.Fatalf("类型不符覆盖错误=%v，期望 E_CONFLICT", perr)
	}
}

// TestCopyRejectsUnsafe 自身子树、根目录、缺失源与缺失父目录都拒绝。
func TestCopyRejectsUnsafe(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "d", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, root, "d/a.txt", "x")
	cases := []struct {
		params string
		code   string
	}{
		{`{"src":"d","dst":"d/sub/x"}`, pluginapi.E_INVALID},
		{`{"src":".","dst":"x"}`, pluginapi.E_INVALID},
		{`{"src":"d","dst":"d"}`, pluginapi.E_INVALID},
		{`{"src":"missing","dst":"x"}`, pluginapi.E_NOTFOUND},
		{`{"src":"d","dst":"nope/x"}`, pluginapi.E_NOTFOUND},
		{`{"src":"d","dst":"../x"}`, pluginapi.E_INVALID},
		{`{"src":"d","dst":".trash/x"}`, pluginapi.E_INVALID},
	}
	for _, c := range cases {
		_, perr := Call("file.copy", json.RawMessage(c.params), root, nil)
		if perr == nil || perr.Code != c.code {
			t.Fatalf("%s 错误=%v，期望 %s", c.params, perr, c.code)
		}
	}
}
