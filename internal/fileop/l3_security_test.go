package fileop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// TestL3SecurityLexicalReject 新增方法在词法层拒绝穿越、绝对路径、反斜杠与 NUL。
func TestL3SecurityLexicalReject(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "a.txt", "x")
	cases := []struct{ name, method, params string }{
		{"search-up", "file.search", `{"path":"../../etc","pattern":"*"}`},
		{"search-abs", "file.search", `{"path":"/etc","pattern":"*"}`},
		{"copy-src-up", "file.copy", `{"src":"../../etc/passwd","dst":"x"}`},
		{"copy-dst-up", "file.copy", `{"src":"a.txt","dst":"../x"}`},
		{"copy-dst-abs", "file.copy", `{"src":"a.txt","dst":"/tmp/x"}`},
		{"rename-src-up", "file.rename", `{"path":"../a.txt","name":"b.txt"}`},
		{"rename-dst-slash", "file.rename", `{"path":"a.txt","name":"../b.txt"}`},
		{"rename-nul", "file.rename", `{"path":"a.txt","name":"a\u0000b"}`},
		{"trash-restore-slash", "file.trash.restore", `{"id":"../../x"}`},
		{"trash-purge-slash", "file.trash.purge", `{"id":"../x"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { assertInvalid(t, c.method, c.params, root, "") })
	}
	// 批量方法的拒绝发生在条目级：整体成功帧，逐项 E_INVALID 且不回显宿主路径。
	assertBatchInvalid(t, root, `{"op":"delete","items":[{"path":"../x","permanent":true}]}`, "")
}

// TestL3SecuritySymlinkEscape 新方法对指向根外的符号链接一律拒绝（含父目录劫持）。
func TestL3SecuritySymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(outside, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "dir", "secret"), []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "out-link")); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ name, method, params string }{
		{"search-root-link", "file.search", `{"path":"out-link","pattern":"*"}`},
		{"search-through", "file.search", `{"path":"out-link/dir","pattern":"*"}`},
		{"copy-src-link", "file.copy", `{"src":"out-link/dir/secret","dst":"x"}`},
		{"copy-dst-link", "file.copy", `{"src":"out-link/dir/secret","dst":"out-link/copied"}`},
		{"rename-src-link", "file.rename", `{"path":"out-link/dir/secret","name":"x"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { assertInvalid(t, c.method, c.params, root, outside) })
	}
	assertBatchInvalid(t, root, `{"op":"copy","items":[{"src":"out-link/dir/secret","dst":"x"}]}`, outside)
	if _, err := os.Stat(filepath.Join(outside, "copied")); !os.IsNotExist(err) {
		t.Fatalf("复制到根外成功，安全防线被绕过: %v", err)
	}
}

// TestL3SecurityTrashNotEscapePort 回收站内部路径不能作为恢复目标或批量目标的跳板。
func TestL3SecurityTrashNotEscapePort(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "a.txt", "x")
	var deleted DeleteResult
	if perr := mustCall(t, "file.delete", `{"path":"a.txt"}`, root, &deleted); perr != nil {
		t.Fatal(perr)
	}
	// 直接构造一个指向根外的 meta.json，恢复时必须被 resolveDestination 拦住。
	entryDir := filepath.Join(root, TrashDirName, "entries", deleted.TrashID)
	if err := os.WriteFile(filepath.Join(entryDir, trashMetaName),
		[]byte(`{"id":"`+deleted.TrashID+`","path":"../escape","kind":"file","size":1,"deleted_at":"2026-01-01T00:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, perr := Call("file.trash.restore", json.RawMessage(`{"id":"`+deleted.TrashID+`"}`), root, nil)
	if perr == nil || perr.Code != pluginapi.E_INVALID {
		t.Fatalf("恶意 meta 恢复错误=%v，期望 E_INVALID", perr)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escape")); !os.IsNotExist(err) {
		t.Fatalf("文件被恢复到根外: %v", err)
	}
}

// TestL3SecurityTrashCorruptMeta 损坏的 meta 不会让列表整体失败，但不可恢复。
func TestL3SecurityTrashCorruptMeta(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "a.txt", "x")
	if err := os.MkdirAll(filepath.Join(root, TrashDirName, "entries", "0000000000000000001-deadbeef"), 0o700); err != nil {
		t.Fatal(err)
	}
	entryDir := filepath.Join(root, TrashDirName, "entries", "0000000000000000001-deadbeef")
	if err := os.WriteFile(filepath.Join(entryDir, trashMetaName), []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entryDir, trashDataName), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	var listing TrashListResult
	if perr := mustCall(t, "file.trash.list", `{}`, root, &listing); perr != nil {
		t.Fatalf("列表不应因损坏条目失败: %v", perr)
	}
	if listing.Count != 0 || listing.Skipped != 1 {
		t.Fatalf("损坏条目应计入 skipped: %+v", listing)
	}
}

// assertBatchInvalid 断言批量请求的每条条目都被安全拒绝（批量是逐项结果，不是整体报错）。
func assertBatchInvalid(t *testing.T, root, params, outside string) {
	t.Helper()
	data, perr := Call("file.batch", json.RawMessage(params), root, nil)
	if perr != nil {
		t.Fatalf("批量结构性错误不应出现: %v", perr)
	}
	var got BatchResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("批量结果解码失败: %v", err)
	}
	if got.Failed != got.Total || got.Total == 0 {
		t.Fatalf("批量条目应全部被拒绝: %+v", got)
	}
	for _, item := range got.Items {
		if item.OK || item.Error == nil || item.Error.Code != pluginapi.E_INVALID {
			t.Fatalf("条目未被安全拒绝: %+v", item)
		}
		if outside != "" && strings.Contains(item.Error.Message, outside) {
			t.Fatalf("批量错误信息泄露宿主路径: %q", item.Error.Message)
		}
	}
}
