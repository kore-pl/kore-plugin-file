package fileop

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// TestWriteCreateAndOverwrite 新建后覆盖：created 翻转、内容替换、mod_time 可解析。
func TestWriteCreateAndOverwrite(t *testing.T) {
	root := t.TempDir()
	var created WriteResult
	if perr := mustCall(t, "file.write", `{"path":"a.txt","content":"first"}`, root, &created); perr != nil {
		t.Fatalf("新建写入失败: %v", perr)
	}
	if !created.Created || created.Size != 5 || created.Path != "a.txt" {
		t.Fatalf("新建结果异常: %+v", created)
	}
	if _, err := time.Parse(time.RFC3339, created.ModTime); err != nil {
		t.Fatalf("mod_time 不是 RFC3339: %q", created.ModTime)
	}
	var overwritten WriteResult
	if perr := mustCall(t, "file.write", `{"path":"a.txt","content":"second"}`, root, &overwritten); perr != nil {
		t.Fatalf("覆盖写入失败: %v", perr)
	}
	if overwritten.Created || overwritten.Size != 6 {
		t.Fatalf("覆盖结果异常: %+v", overwritten)
	}
	data, err := os.ReadFile(filepath.Join(root, "a.txt"))
	if err != nil || string(data) != "second" {
		t.Fatalf("落盘内容=%q err=%v", data, err)
	}
}

// TestWriteParentCreate 父目录缺失：默认 E_NOTFOUND，create_if_missing=true 时自动建目录。
func TestWriteParentCreate(t *testing.T) {
	root := t.TempDir()
	_, perr := Call("file.write", json.RawMessage(`{"path":"sub/deep/a.txt","content":"x"}`), root, nil)
	if perr == nil || perr.Code != pluginapi.E_NOTFOUND || !strings.Contains(perr.Message, "create_if_missing") {
		t.Fatalf("父目录缺失错误=%v", perr)
	}
	if perr := mustCall(t, "file.write", `{"path":"sub/deep/a.txt","content":"x","create_if_missing":true}`, root, nil); perr != nil {
		t.Fatalf("自动建父目录失败: %v", perr)
	}
	if _, err := os.Stat(filepath.Join(root, "sub", "deep", "a.txt")); err != nil {
		t.Fatalf("目标文件未落盘: %v", err)
	}
}

// TestWritePreservesMode 覆盖已有文件时保留原权限位。
func TestWritePreservesMode(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if perr := mustCall(t, "file.write", `{"path":"a.txt","content":"new"}`, root, nil); perr != nil {
		t.Fatalf("写入失败: %v", perr)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("权限=%v err=%v，期望 0600", info.Mode().Perm(), err)
	}
}

// TestWriteAtomicOnRenameFailure rename 失败时目标保持原内容、目录不留临时半文件。
func TestWriteAtomicOnRenameFailure(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldRename := renameFile
	renameFile = func(string, string) error { return errors.New("injected rename failure") }
	defer func() { renameFile = oldRename }()

	_, perr := Call("file.write", json.RawMessage(`{"path":"a.txt","content":"new"}`), root, nil)
	if perr == nil || perr.Code != pluginapi.E_INTERNAL {
		t.Fatalf("rename 失败错误=%v，期望 E_INTERNAL", perr)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "old" {
		t.Fatalf("失败后目标内容=%q err=%v，期望保持 old", data, err)
	}
	leftovers, err := filepath.Glob(filepath.Join(root, ".kore-file-*.tmp"))
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("残留临时文件: %v err=%v", leftovers, err)
	}
}

// TestWriteNoTempLeftover 成功写入后目录里不留临时文件。
func TestWriteNoTempLeftover(t *testing.T) {
	root := t.TempDir()
	if perr := mustCall(t, "file.write", `{"path":"a.txt","content":"ok"}`, root, nil); perr != nil {
		t.Fatalf("写入失败: %v", perr)
	}
	leftovers, err := filepath.Glob(filepath.Join(root, ".kore-file-*.tmp"))
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("残留临时文件: %v err=%v", leftovers, err)
	}
}

// TestWriteLimit 内容超过 8 MiB 返回 E_INVALID；写入目录返回 E_CONFLICT。
func TestWriteLimit(t *testing.T) {
	root := t.TempDir()
	_, perr := Call("file.write", json.RawMessage(`{"path":"a.txt","content":"`+strings.Repeat("x", MaxWriteBytes+1)+`"}`), root, nil)
	if perr == nil || perr.Code != pluginapi.E_INVALID {
		t.Fatalf("超限写入错误=%v，期望 E_INVALID", perr)
	}
	if _, perr := Call("file.write", json.RawMessage(`{"path":".","content":"x"}`), root, nil); perr == nil || perr.Code != pluginapi.E_CONFLICT {
		t.Fatalf("写目录错误=%v，期望 E_CONFLICT", perr)
	}
}
