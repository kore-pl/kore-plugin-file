package fileop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// jsonRaw 是测试里构造 params 的简写。
func jsonRaw(s string) json.RawMessage { return json.RawMessage(s) }

// TestMoveRenameSameDir 同目录改名：moved=true + method=rename。
func TestMoveRenameSameDir(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	var got MoveResult
	if perr := mustCall(t, "file.move", `{"src":"a.txt","dst":"b.txt"}`, root, &got); perr != nil {
		t.Fatalf("file.move 失败: %v", perr)
	}
	if !got.Moved || got.Method != "rename" || got.From != "a.txt" || got.To != "b.txt" {
		t.Fatalf("移动结果异常: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(root, "a.txt")); !os.IsNotExist(err) {
		t.Fatalf("源文件仍存在: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "b.txt")); err != nil || string(data) != "content" {
		t.Fatalf("目标内容=%q err=%v", data, err)
	}
}

// TestMoveAcrossDirs 跨目录移动目录树，内容保持。
func TestMoveAcrossDirs(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "dst"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "sub", "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var got MoveResult
	if perr := mustCall(t, "file.move", `{"src":"src","dst":"dst/src"}`, root, &got); perr != nil {
		t.Fatalf("跨目录移动失败: %v", perr)
	}
	if data, err := os.ReadFile(filepath.Join(root, "dst", "src", "sub", "f.txt")); err != nil || string(data) != "x" {
		t.Fatalf("移动后内容=%q err=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(root, "src")); !os.IsNotExist(err) {
		t.Fatalf("源目录仍存在: %v", err)
	}
}

// TestMoveConflictAndOverwrite 目标存在：默认 E_CONFLICT，overwrite=true 覆盖。
func TestMoveConflictAndOverwrite(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, perr := Call("file.move", jsonRaw(`{"src":"a.txt","dst":"b.txt"}`), root, nil)
	if perr == nil || perr.Code != pluginapi.E_CONFLICT {
		t.Fatalf("冲突移动错误=%v，期望 E_CONFLICT", perr)
	}
	var got MoveResult
	if perr := mustCall(t, "file.move", `{"src":"a.txt","dst":"b.txt","overwrite":true}`, root, &got); perr != nil {
		t.Fatalf("覆盖移动失败: %v", perr)
	}
	if data, err := os.ReadFile(filepath.Join(root, "b.txt")); err != nil || string(data) != "new" {
		t.Fatalf("覆盖后内容=%q err=%v", data, err)
	}
}

// TestMoveRejectsUnsafeTargets 移入自身子目录、缺源、缺父目录、非法目标都拒绝。
func TestMoveRejectsUnsafeTargets(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "d", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		params string
		code   string
	}{
		{`{"src":"d","dst":"d/sub/x"}`, pluginapi.E_INVALID},
		{`{"src":"missing","dst":"x"}`, pluginapi.E_NOTFOUND},
		{`{"src":"d","dst":"nope/x"}`, pluginapi.E_NOTFOUND},
		{`{"src":".","dst":"x"}`, pluginapi.E_INVALID},
		{`{"src":"d","dst":"d"}`, pluginapi.E_INVALID},
		{`{"src":"d","dst":"../x"}`, pluginapi.E_INVALID},
	}
	for _, c := range cases {
		_, perr := Call("file.move", jsonRaw(c.params), root, nil)
		if perr == nil || perr.Code != c.code {
			t.Fatalf("%s 错误=%v，期望 %s", c.params, perr, c.code)
		}
	}
}

// TestMoveCrossDeviceFallback 模拟 EXDEV：回退为复制 + 删除，源消失、内容保留。
func TestMoveCrossDeviceFallback(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("cross-device"), 0o640); err != nil {
		t.Fatal(err)
	}
	original := renamePath
	renamePath = func(string, string) error {
		return &os.LinkError{Op: "rename", Err: syscall.EXDEV}
	}
	defer func() { renamePath = original }()
	// overwrite=false 的主落名走原子路径；这里只对源文件名注入 EXDEV，
	// 临时项 → 目标的最终落名仍走真实 renameNoReplace（保持「不覆盖」语义）。
	originalNoReplace := renameNoReplacePath
	renameNoReplacePath = func(src, dst string) error {
		if src == filepath.Join(root, "a.txt") {
			return &os.LinkError{Op: "rename", Err: syscall.EXDEV}
		}
		return originalNoReplace(src, dst)
	}
	defer func() { renameNoReplacePath = originalNoReplace }()
	var got MoveResult
	if perr := mustCall(t, "file.move", `{"src":"a.txt","dst":"b.txt"}`, root, &got); perr != nil {
		t.Fatalf("跨设备回退失败: %v", perr)
	}
	if got.Method != "copy+delete" {
		t.Fatalf("method=%q，期望 copy+delete", got.Method)
	}
	if data, err := os.ReadFile(filepath.Join(root, "b.txt")); err != nil || string(data) != "cross-device" {
		t.Fatalf("回退后内容=%q err=%v", data, err)
	}
	if info, err := os.Stat(filepath.Join(root, "b.txt")); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("回退后权限=%v err=%v，期望 0640", info.Mode().Perm(), err)
	}
	if _, err := os.Stat(filepath.Join(root, "a.txt")); !os.IsNotExist(err) {
		t.Fatalf("回退后源文件仍存在: %v", err)
	}
}
