//go:build unix

package fileop

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveRootDefault 验证缺省根收口：空值回落整机 "/"，显式值原样保留。
func TestResolveRootDefault(t *testing.T) {
	cases := map[string]string{"": DefaultRoot, "   ": DefaultRoot, "/var/www": "/var/www", "/": "/"}
	for raw, want := range cases {
		if got := ResolveRoot(raw); got != want {
			t.Fatalf("ResolveRoot(%q)=%q，期望 %q", raw, got, want)
		}
	}
	if DefaultRoot != "/" {
		t.Fatalf("DefaultRoot=%q，期望整机根 /", DefaultRoot)
	}
}

// TestListWholeDiskDefaultRoot 验证缺省根下 file.list 能列出真实目录：
// 根目录有 /etc，进入 etc 后仍有条目且带权限位/属主字段。
func TestListWholeDiskDefaultRoot(t *testing.T) {
	var root ListResult
	if perr := mustCall(t, "file.list", `{"path":"."}`, DefaultRoot, &root); perr != nil {
		t.Fatalf("整机根 file.list 失败: %v", perr)
	}
	if root.Count == 0 || len(root.Entries) == 0 {
		t.Fatalf("整机根不应为空: %+v", root)
	}
	names := map[string]bool{}
	for _, e := range root.Entries {
		names[e.Name] = true
		if e.Mode == "" {
			t.Fatalf("条目 %s 缺权限位: %+v", e.Name, e)
		}
		if e.ModTime == "" {
			t.Fatalf("条目 %s 缺修改时间: %+v", e.Name, e)
		}
	}
	if !names["etc"] {
		t.Fatalf("整机根缺少 etc 目录，条目=%v", names)
	}
	var etc ListResult
	if perr := mustCall(t, "file.list", `{"path":"etc"}`, DefaultRoot, &etc); perr != nil {
		t.Fatalf("file.list etc 失败: %v", perr)
	}
	if etc.Count == 0 {
		t.Fatalf("etc 不应为空")
	}
}

// TestListEntryCarriesOwner 验证条目 JSON 带 uid/gid 字段（属主列的数据来源）。
func TestListEntryCarriesOwner(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o640); err != nil {
		t.Fatal(err)
	}
	raw, perr := Call("file.list", json.RawMessage(`{"path":"."}`), root, nil)
	if perr != nil {
		t.Fatalf("file.list 失败: %v", perr)
	}
	if !strings.Contains(string(raw), `"uid":`) || !strings.Contains(string(raw), `"gid":`) {
		t.Fatalf("列表结果缺 uid/gid: %s", raw)
	}
}

// TestTrashFallbackWholeDiskRoot 验证整机根下删除走可写兜底回收站，且能恢复：
// 低权进程无法在 / 下建 /.trash，不兜底时删除必然失败。
func TestTrashFallbackWholeDiskRoot(t *testing.T) {
	trashDir := t.TempDir()
	t.Setenv("KORE_FILE_TRASH", trashDir)
	work := t.TempDir()
	file := filepath.Join(work, "note.txt")
	if err := os.WriteFile(file, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel("/", file)
	if err != nil {
		t.Fatal(err)
	}
	var del DeleteResult
	if perr := mustCall(t, "file.delete", fmt.Sprintf(`{"path":%q}`, rel), DefaultRoot, &del); perr != nil {
		t.Fatalf("整机根删除失败: %v", perr)
	}
	if del.TrashID == "" {
		t.Fatalf("删除未进回收站: %+v", del)
	}
	if _, err := os.Stat(filepath.Join(trashDir, trashEntriesDirName)); err != nil {
		t.Fatalf("兜底回收站未创建: %v", err)
	}
	var list TrashListResult
	if perr := mustCall(t, "file.trash.list", `{}`, DefaultRoot, &list); perr != nil {
		t.Fatalf("回收站列表失败: %v", perr)
	}
	if list.Total != 1 {
		t.Fatalf("回收站应有 1 项，实际 %d", list.Total)
	}
	var res TrashRestoreResult
	if perr := mustCall(t, "file.trash.restore", fmt.Sprintf(`{"id":%q}`, del.TrashID), DefaultRoot, &res); perr != nil {
		t.Fatalf("恢复失败: %v", perr)
	}
	got, err := os.ReadFile(file)
	if err != nil || string(got) != "hi" {
		t.Fatalf("恢复内容异常: %q err=%v", got, err)
	}
}
