package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pengtang1024/kore-plugin-file/internal/fileop"
	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// TestFilePluginL3E2E 真实二进制 + UDS：rename / copy / search / 回收站往返 / 批量部分失败。
func TestFilePluginL3E2E(t *testing.T) {
	bin := buildPlugin(t)
	root := t.TempDir()
	mustWriteFile(t, root, "a.txt", "alpha\nsecond line\n")
	mustWriteFile(t, root, "b.txt", "beta\n")
	session := startPlugin(t, bin, root)

	var renamed fileop.RenameResult
	decode(t, session.must("file.rename", `{"path":"a.txt","name":"a2.txt"}`), &renamed)
	if !renamed.Renamed || renamed.To != "a2.txt" {
		t.Fatalf("rename 结果异常: %+v", renamed)
	}
	if _, perr := session.call("file.stat", `{"path":"a.txt"}`); perr == nil || perr.Code != pluginapi.E_NOTFOUND {
		t.Fatalf("旧路径 stat 错误=%v，期望 E_NOTFOUND", perr)
	}

	var copied fileop.CopyResult
	decode(t, session.must("file.copy", `{"src":"a2.txt","dst":"copy.txt"}`), &copied)
	if !copied.Copied || copied.Bytes != int64(len("alpha\nsecond line\n")) {
		t.Fatalf("copy 结果异常: %+v", copied)
	}

	var found fileop.SearchResult
	decode(t, session.must("file.search", `{"path":".","pattern":"*.txt","content":"second"}`), &found)
	if found.Matched != 2 {
		t.Fatalf("内容搜索命中=%d，期望 2（a2.txt 与 copy.txt）: %+v", found.Matched, found.Entries)
	}
	for _, entry := range found.Entries {
		if entry.Line != 2 {
			t.Fatalf("命中行号=%d，期望 2: %+v", entry.Line, entry)
		}
	}

	// 删除进回收站：原路径消失、回收站可见、恢复后内容一致。
	var deleted fileop.DeleteResult
	decode(t, session.must("file.delete", `{"path":"b.txt"}`), &deleted)
	if deleted.TrashID == "" || deleted.Permanent {
		t.Fatalf("delete 未进入回收站: %+v", deleted)
	}
	var listing fileop.TrashListResult
	decode(t, session.must("file.trash.list", `{}`), &listing)
	if listing.Count != 1 || listing.Entries[0].Path != "b.txt" {
		t.Fatalf("回收站列表异常: %+v", listing)
	}
	var restored fileop.TrashRestoreResult
	decode(t, session.must("file.trash.restore", `{"id":"`+deleted.TrashID+`"}`), &restored)
	if !restored.Restored {
		t.Fatalf("恢复结果异常: %+v", restored)
	}
	if data, err := os.ReadFile(filepath.Join(root, "b.txt")); err != nil || string(data) != "beta\n" {
		t.Fatalf("恢复后内容=%q err=%v", data, err)
	}

	// 批量部分失败：一个缺失不影响其余；所有条目都仍有回收站 id 时可直接恢复。
	batch := `{"op":"delete","items":[{"path":"copy.txt"},{"path":"nope.txt"},{"path":"b.txt"}]}`
	var batched fileop.BatchResult
	decode(t, session.must("file.batch", batch), &batched)
	if batched.Total != 3 || batched.Succeeded != 2 || batched.Failed != 1 {
		t.Fatalf("批量结果异常: %+v", batched)
	}
	if batched.Items[1].Error == nil || batched.Items[1].Error.Code != pluginapi.E_NOTFOUND {
		t.Fatalf("失败项错误异常: %+v", batched.Items[1])
	}
	var purged fileop.TrashPurgeResult
	decode(t, session.must("file.trash.purge", `{"all":true}`), &purged)
	if purged.Purged != 2 {
		t.Fatalf("清空回收站条数=%d，期望 2", purged.Purged)
	}
	if _, perr := session.call("file.trash.restore", `{"id":"`+deleted.TrashID+`"}`); perr == nil || perr.Code != pluginapi.E_NOTFOUND {
		t.Fatalf("清除后恢复错误=%v，期望 E_NOTFOUND", perr)
	}

	// 路径穿越与回收站保留路径在真实 UDS 链路同样被拒绝。
	if _, perr := session.call("file.copy", `{"src":"a2.txt","dst":"../escape.txt"}`); perr == nil || perr.Code != pluginapi.E_INVALID {
		t.Fatalf("穿越复制错误=%v，期望 E_INVALID", perr)
	}
	if _, perr := session.call("file.write", `{"path":".trash/x","content":"y"}`); perr == nil || perr.Code != pluginapi.E_INVALID {
		t.Fatalf("写回收站错误=%v，期望 E_INVALID", perr)
	}
	session.shutdown()
}

// mustWriteFile 在 E2E 根目录造一个真实文件（父目录自动创建）。
func mustWriteFile(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
