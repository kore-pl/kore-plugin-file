package fileop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// TestSearchByNameGlob 文件名 glob 命中：返回相对路径、目录也参与匹配。
func TestSearchByNameGlob(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "a.txt", "a")
	mustWrite(t, root, "b.log", "b")
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, root, "sub/c.txt", "c")
	var got SearchResult
	if perr := mustCall(t, "file.search", `{"path":".","pattern":"*.txt"}`, root, &got); perr != nil {
		t.Fatalf("搜索失败: %v", perr)
	}
	if got.Matched != 2 || got.Truncated {
		t.Fatalf("命中数=%d truncated=%v，期望 2/false", got.Matched, got.Truncated)
	}
	paths := map[string]bool{}
	for _, entry := range got.Entries {
		paths[entry.Path] = true
		if entry.Line != 0 || entry.Preview != "" {
			t.Fatalf("文件名命中不应带行信息: %+v", entry)
		}
	}
	if !paths["a.txt"] || !paths["sub/c.txt"] || paths["b.log"] {
		t.Fatalf("命中路径异常: %+v", paths)
	}
}

// TestSearchByContent 内容搜索：只有含关键词的行命中，行号与预览可用。
func TestSearchByContent(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.log"), []byte("line1\nline2 needle here\nline3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, root, "other.log", "nothing")
	var got SearchResult
	if perr := mustCall(t, "file.search", `{"path":".","pattern":"*.log","content":"needle"}`, root, &got); perr != nil {
		t.Fatalf("内容搜索失败: %v", perr)
	}
	if got.Matched != 1 {
		t.Fatalf("内容命中数=%d，期望 1: %+v", got.Matched, got.Entries)
	}
	entry := got.Entries[0]
	if entry.Path != "app.log" || entry.Line != 2 || !strings.Contains(entry.Preview, "needle") {
		t.Fatalf("内容命中异常: %+v", entry)
	}
	var upper SearchResult
	if perr := mustCall(t, "file.search", `{"path":".","pattern":"*.log","content":"NEEDLE","ignore_case":true}`, root, &upper); perr != nil {
		t.Fatalf("忽略大小写搜索失败: %v", perr)
	}
	if upper.Matched != 1 {
		t.Fatalf("忽略大小写命中数=%d，期望 1", upper.Matched)
	}
}

// TestSearchSkipsLargeAndBinary 超体积与二进制文件不读进内存：只计入 skipped。
func TestSearchSkipsLargeAndBinary(t *testing.T) {
	root := t.TempDir()
	large := append([]byte("needle\n"), make([]byte, 4096)...)
	if err := os.WriteFile(filepath.Join(root, "large.log"), large, 0o644); err != nil {
		t.Fatal(err)
	}
	binary := append([]byte("needle\x00"), make([]byte, 16)...)
	if err := os.WriteFile(filepath.Join(root, "bin.dat"), binary, 0o644); err != nil {
		t.Fatal(err)
	}
	var got SearchResult
	params := `{"path":".","pattern":"*","content":"needle","max_file_bytes":1024}`
	if perr := mustCall(t, "file.search", params, root, &got); perr != nil {
		t.Fatalf("搜索失败: %v", perr)
	}
	if got.Matched != 0 {
		t.Fatalf("不应有命中: %+v", got.Entries)
	}
	if got.Skipped != 2 {
		t.Fatalf("skipped=%d，期望 2（超体积 + 二进制）", got.Skipped)
	}
}

// TestSearchTruncatedResults 结果上限触发 truncated，并且不再继续遍历。
func TestSearchTruncatedResults(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt", "d.txt"} {
		mustWrite(t, root, name, "x")
	}
	var got SearchResult
	if perr := mustCall(t, "file.search", `{"path":".","pattern":"*.txt","max_results":2}`, root, &got); perr != nil {
		t.Fatalf("搜索失败: %v", perr)
	}
	if got.Matched != 2 || !got.Truncated {
		t.Fatalf("结果=%d truncated=%v，期望 2/true", got.Matched, got.Truncated)
	}
}

// TestSearchSkipsTrash 回收站不参与搜索，普通方法也不能搜进去。
func TestSearchSkipsTrash(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "a.txt", "x")
	trashFile := filepath.Join(root, TrashDirName, "entries", "hidden", "data")
	if err := os.MkdirAll(filepath.Dir(trashFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trashFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	var got SearchResult
	if perr := mustCall(t, "file.search", `{"path":".","pattern":"*"}`, root, &got); perr != nil {
		t.Fatalf("搜索失败: %v", perr)
	}
	for _, entry := range got.Entries {
		if strings.HasPrefix(entry.Path, TrashDirName+"/") {
			t.Fatalf("回收站条目泄露到搜索结果: %+v", entry)
		}
	}
	if _, perr := Call("file.search", json.RawMessage(`{"path":".trash","pattern":"*"}`), root, nil); perr == nil || perr.Code != pluginapi.E_INVALID {
		t.Fatalf("搜索回收站错误=%v，期望 E_INVALID", perr)
	}
}

// TestSearchParamValidation 非法 glob、越界阈值、非法根都返回 E_INVALID / E_NOTFOUND。
func TestSearchParamValidation(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "a.txt", "x")
	cases := []struct {
		params string
		code   string
	}{
		{`{"pattern":""}`, pluginapi.E_INVALID},
		{`{"pattern":"[bad"}`, pluginapi.E_INVALID},
		{`{"pattern":"*","max_results":-1}`, pluginapi.E_INVALID},
		{`{"pattern":"*","max_results":1001}`, pluginapi.E_INVALID},
		{`{"pattern":"*","max_file_bytes":8388609}`, pluginapi.E_INVALID},
		{`{"pattern":"*","path":"missing"}`, pluginapi.E_NOTFOUND},
		{`{"pattern":"*","path":"a.txt"}`, pluginapi.E_INVALID},
		{`{"pattern":"*","path":"../../etc"}`, pluginapi.E_INVALID},
	}
	for _, c := range cases {
		_, perr := Call("file.search", json.RawMessage(c.params), root, nil)
		if perr == nil || perr.Code != c.code {
			t.Fatalf("%s 错误=%v，期望 %s", c.params, perr, c.code)
		}
	}
}
