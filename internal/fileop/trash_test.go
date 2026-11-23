package fileop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// TestTrashRoundTrip 删除 → 列出 → 恢复 → 内容一致。
func TestTrashRoundTrip(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "a.txt", "keep-me")
	var deleted DeleteResult
	if perr := mustCall(t, "file.delete", `{"path":"a.txt"}`, root, &deleted); perr != nil {
		t.Fatalf("删除失败: %v", perr)
	}
	if !deleted.Deleted || deleted.Permanent || deleted.TrashID == "" {
		t.Fatalf("删除结果异常: %+v", deleted)
	}
	if _, err := os.Stat(filepath.Join(root, "a.txt")); !os.IsNotExist(err) {
		t.Fatalf("文件仍留在原处: %v", err)
	}
	var listing TrashListResult
	if perr := mustCall(t, "file.trash.list", `{}`, root, &listing); perr != nil {
		t.Fatalf("列出回收站失败: %v", perr)
	}
	if listing.Count != 1 || listing.Entries[0].ID != deleted.TrashID || listing.Entries[0].Path != "a.txt" {
		t.Fatalf("回收站列表异常: %+v", listing)
	}
	if listing.Entries[0].Kind != "file" || listing.Entries[0].Size != 7 {
		t.Fatalf("条目元数据异常: %+v", listing.Entries[0])
	}
	var restored TrashRestoreResult
	if perr := mustCall(t, "file.trash.restore", `{"id":"`+deleted.TrashID+`"}`, root, &restored); perr != nil {
		t.Fatalf("恢复失败: %v", perr)
	}
	if !restored.Restored || restored.Path != "a.txt" {
		t.Fatalf("恢复结果异常: %+v", restored)
	}
	if data, err := os.ReadFile(filepath.Join(root, "a.txt")); err != nil || string(data) != "keep-me" {
		t.Fatalf("恢复后内容=%q err=%v", data, err)
	}
	if perr := mustCall(t, "file.trash.list", `{}`, root, &listing); perr != nil || listing.Count != 0 {
		t.Fatalf("恢复后回收站应为空: %+v err=%v", listing, perr)
	}
}

// TestTrashRestoreDirTree 目录树往返：结构与内容逐一保持。
func TestTrashRestoreDirTree(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "dir", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, root, "dir/sub/f.txt", "nested")
	mustWrite(t, root, "dir/g.txt", "g")
	var deleted DeleteResult
	if perr := mustCall(t, "file.delete", `{"path":"dir","recursive":true}`, root, &deleted); perr != nil {
		t.Fatalf("删除目录失败: %v", perr)
	}
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatalf("原路径应可重新创建: %v", err)
	}
	if perr := mustCall(t, "file.trash.restore", `{"id":"`+deleted.TrashID+`"}`, root, nil); perr == nil || perr.Code != pluginapi.E_CONFLICT {
		t.Fatalf("原路径已存在时应回 E_CONFLICT，实际: %v", perr)
	}
	if err := os.Remove(filepath.Join(root, "dir")); err != nil {
		t.Fatal(err)
	}
	if perr := mustCall(t, "file.trash.restore", `{"id":"`+deleted.TrashID+`"}`, root, nil); perr != nil {
		t.Fatalf("恢复目录失败: %v", perr)
	}
	if data, err := os.ReadFile(filepath.Join(root, "dir", "sub", "f.txt")); err != nil || string(data) != "nested" {
		t.Fatalf("恢复后嵌套内容=%q err=%v", data, err)
	}
}

// TestTrashPermanentAndPurge permanent=true 直接删；purge 后不可恢复。
func TestTrashPermanentAndPurge(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "a.txt", "x")
	var deleted DeleteResult
	if perr := mustCall(t, "file.delete", `{"path":"a.txt","permanent":true}`, root, &deleted); perr != nil {
		t.Fatalf("直接删除失败: %v", perr)
	}
	if !deleted.Permanent || deleted.TrashID != "" {
		t.Fatalf("直接删除结果异常: %+v", deleted)
	}
	var listing TrashListResult
	if perr := mustCall(t, "file.trash.list", `{}`, root, &listing); perr != nil || listing.Count != 0 {
		t.Fatalf("直接删除不应产生回收站条目: %+v err=%v", listing, perr)
	}
	mustWrite(t, root, "b.txt", "y")
	mustWrite(t, root, "c.txt", "z")
	var first, second DeleteResult
	if perr := mustCall(t, "file.delete", `{"path":"b.txt"}`, root, &first); perr != nil {
		t.Fatal(perr)
	}
	if perr := mustCall(t, "file.delete", `{"path":"c.txt"}`, root, &second); perr != nil {
		t.Fatal(perr)
	}
	var purged TrashPurgeResult
	if perr := mustCall(t, "file.trash.purge", `{"id":"`+first.TrashID+`"}`, root, &purged); perr != nil {
		t.Fatalf("清除单条失败: %v", perr)
	}
	if purged.Purged != 1 || purged.All {
		t.Fatalf("清除单条结果异常: %+v", purged)
	}
	if perr := mustCall(t, "file.trash.restore", `{"id":"`+first.TrashID+`"}`, root, nil); perr == nil || perr.Code != pluginapi.E_NOTFOUND {
		t.Fatalf("清除后恢复应回 E_NOTFOUND，实际: %v", perr)
	}
	var purgedAll TrashPurgeResult
	if perr := mustCall(t, "file.trash.purge", `{"all":true}`, root, &purgedAll); perr != nil {
		t.Fatalf("清空回收站失败: %v", perr)
	}
	if purgedAll.Purged != 1 || !purgedAll.All {
		t.Fatalf("清空结果异常: %+v", purgedAll)
	}
	if perr := mustCall(t, "file.trash.restore", `{"id":"`+second.TrashID+`"}`, root, nil); perr == nil || perr.Code != pluginapi.E_NOTFOUND {
		t.Fatalf("清空后恢复应回 E_NOTFOUND，实际: %v", perr)
	}
}

// TestTrashRejectsBadRequests 空参数、id+all 冲突、非法 id、非空目录语义都按契约拒绝。
func TestTrashRejectsBadRequests(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "a.txt", "x")
	cases := []struct {
		params string
		code   string
	}{
		{`{"id":""}`, pluginapi.E_INVALID},
		{`{"all":true,"id":"x"}`, pluginapi.E_INVALID},
		{`{"id":"../../etc/passwd"}`, pluginapi.E_INVALID},
		{`{"id":"..%2f.."}`, pluginapi.E_INVALID},
	}
	for _, c := range cases {
		_, perr := Call("file.trash.restore", json.RawMessage(c.params), root, nil)
		if perr == nil || perr.Code != c.code {
			t.Fatalf("trash.restore %s 错误=%v，期望 %s", c.params, perr, c.code)
		}
	}
	if _, perr := Call("file.trash.purge", json.RawMessage(`{}`), root, nil); perr == nil || perr.Code != pluginapi.E_INVALID {
		t.Fatalf("空 purge 参数错误=%v，期望 E_INVALID", perr)
	}
	if err := os.MkdirAll(filepath.Join(root, "full"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, root, "full/x.txt", "x")
	if _, perr := Call("file.delete", json.RawMessage(`{"path":"full"}`), root, nil); perr == nil || perr.Code != pluginapi.E_CONFLICT {
		t.Fatalf("非空目录默认删除错误=%v，期望 E_CONFLICT", perr)
	}
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if perr := mustCall(t, "file.delete", `{"path":"empty"}`, root, nil); perr != nil {
		t.Fatalf("空目录删除失败: %v", perr)
	}
}

// TestTrashListWithoutTrash 从未删除过任何东西时列表为空，且只读方法不创建回收站目录。
func TestTrashListWithoutTrash(t *testing.T) {
	root := t.TempDir()
	var got TrashListResult
	if perr := mustCall(t, "file.trash.list", `{}`, root, &got); perr != nil {
		t.Fatalf("空回收站列表失败: %v", perr)
	}
	if got.Count != 0 || got.Total != 0 || got.Entries == nil || len(got.Entries) != 0 {
		t.Fatalf("空回收站结果异常: %+v", got)
	}
	if _, err := os.Lstat(filepath.Join(root, TrashDirName)); !os.IsNotExist(err) {
		t.Fatalf("只读方法不应创建回收站目录: %v", err)
	}
}
