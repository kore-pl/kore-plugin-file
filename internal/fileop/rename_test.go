package fileop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// TestRenameFile 同目录改名：源消失、目标内容保留、结果 method=rename。
func TestRenameFile(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "a.txt", "content")
	var got RenameResult
	if perr := mustCall(t, "file.rename", `{"path":"a.txt","name":"b.txt"}`, root, &got); perr != nil {
		t.Fatalf("改名失败: %v", perr)
	}
	if !got.Renamed || got.Method != "rename" || got.From != "a.txt" || got.To != "b.txt" {
		t.Fatalf("改名结果异常: %+v", got)
	}
	if data, err := os.ReadFile(filepath.Join(root, "b.txt")); err != nil || string(data) != "content" {
		t.Fatalf("改名后内容=%q err=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(root, "a.txt")); !os.IsNotExist(err) {
		t.Fatalf("源文件仍存在: %v", err)
	}
}

// TestRenameDirAndConflict 目录改名可用；目标已存在默认 E_CONFLICT，overwrite=true 覆盖。
func TestRenameDirAndConflict(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "d", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, root, "d/sub/f.txt", "x")
	var renamed RenameResult
	if perr := mustCall(t, "file.rename", `{"path":"d","name":"e"}`, root, &renamed); perr != nil {
		t.Fatalf("目录改名失败: %v", perr)
	}
	if _, err := os.Stat(filepath.Join(root, "e", "sub", "f.txt")); err != nil {
		t.Fatalf("改名后目录树异常: %v", err)
	}
	mustWrite(t, root, "old.txt", "old")
	mustWrite(t, root, "new.txt", "new")
	_, perr := Call("file.rename", json.RawMessage(`{"path":"new.txt","name":"old.txt"}`), root, nil)
	if perr == nil || perr.Code != pluginapi.E_CONFLICT {
		t.Fatalf("冲突改名错误=%v，期望 E_CONFLICT", perr)
	}
	if perr := mustCall(t, "file.rename", `{"path":"new.txt","name":"old.txt","overwrite":true}`, root, nil); perr != nil {
		t.Fatalf("覆盖改名失败: %v", perr)
	}
	if data, err := os.ReadFile(filepath.Join(root, "old.txt")); err != nil || string(data) != "new" {
		t.Fatalf("覆盖后内容=%q err=%v", data, "new")
	}
}

// TestRenameRejectsPathSemantics rename 只能改名、不能换目录：任何路径语义一律 E_INVALID。
func TestRenameRejectsPathSemantics(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "a.txt", "x")
	cases := []string{
		`{"path":"a.txt","name":"sub/b.txt"}`,
		`{"path":"a.txt","name":".."}`,
		`{"path":"a.txt","name":"."}`,
		`{"path":"a.txt","name":".trash"}`,
		`{"path":"a.txt","name":"a\\b"}`,
		`{"path":".","name":"x"}`,
		`{"path":"a.txt","name":"a.txt"}`,
		`{"path":"a.txt","name":""}`,
	}
	for _, params := range cases {
		_, perr := Call("file.rename", json.RawMessage(params), root, nil)
		if perr == nil || perr.Code != pluginapi.E_INVALID {
			t.Fatalf("%s 错误=%v，期望 E_INVALID", params, perr)
		}
	}
	if _, perr := Call("file.rename", json.RawMessage(`{"path":"missing.txt","name":"b.txt"}`), root, nil); perr == nil || perr.Code != pluginapi.E_NOTFOUND {
		t.Fatalf("缺失源错误=%v，期望 E_NOTFOUND", perr)
	}
}

// TestRenameSymlinkEscape 源路径经符号链接逃逸（或链接目标在根外）时拒绝。
func TestRenameSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "dir-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(root, "file-link")); err != nil {
		t.Fatal(err)
	}
	for _, params := range []string{
		`{"path":"dir-link/secret","name":"x"}`,
		`{"path":"file-link","name":"y"}`,
	} {
		_, perr := Call("file.rename", json.RawMessage(params), root, nil)
		if perr == nil || perr.Code != pluginapi.E_INVALID {
			t.Fatalf("%s 错误=%v，期望 E_INVALID", params, perr)
		}
	}
	if _, perr := Call("file.rename", json.RawMessage(`{"path":"missing.txt","name":"b.txt"}`), root, nil); perr == nil || perr.Code != pluginapi.E_NOTFOUND {
		t.Fatalf("缺失源错误=%v，期望 E_NOTFOUND", perr)
	}
}
