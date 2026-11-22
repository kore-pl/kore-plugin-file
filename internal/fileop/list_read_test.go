package fileop

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// mustCall 调用方法；out 非 nil 时把成功结果解码进去，失败返回协议错误。
func mustCall(t *testing.T, method, params, root string, out any) *pluginapi.Error {
	t.Helper()
	data, perr := Call(method, json.RawMessage(params), root, nil)
	if perr != nil {
		return perr
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("%s 结果解码失败: %v（结果=%s）", method, err, data)
		}
	}
	return nil
}

// TestListEmptyDir 空目录返回空数组（不是 null）且 path 为 "."。
func TestListEmptyDir(t *testing.T) {
	root := t.TempDir()
	var got ListResult
	if perr := mustCall(t, "file.list", `{"path":"."}`, root, &got); perr != nil {
		t.Fatalf("file.list 失败: %v", perr)
	}
	if got.Path != "." || got.Count != 0 || got.Entries == nil || len(got.Entries) != 0 {
		t.Fatalf("空目录结果异常: %+v", got)
	}
}

// TestListEntries 文件、子目录、符号链接的元信息逐项断言（真实 ReadDir）。
func TestListEntries(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a.txt", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	var got ListResult
	if perr := mustCall(t, "file.list", `{"path":"."}`, root, &got); perr != nil {
		t.Fatalf("file.list 失败: %v", perr)
	}
	if got.Count != 3 || len(got.Entries) != 3 {
		t.Fatalf("条目数=%d，期望 3: %+v", got.Count, got.Entries)
	}
	byName := map[string]ListEntry{}
	for _, e := range got.Entries {
		byName[e.Name] = e
	}
	fileEntry, ok := byName["a.txt"]
	if !ok || fileEntry.IsDir || fileEntry.Size != 5 || fileEntry.Mode != "0644" || fileEntry.IsSymlink {
		t.Fatalf("a.txt 元信息异常: %+v", fileEntry)
	}
	if fileEntry.Path != "a.txt" {
		t.Fatalf("a.txt 相对路径=%q", fileEntry.Path)
	}
	if _, err := time.Parse(time.RFC3339, fileEntry.ModTime); err != nil {
		t.Fatalf("mod_time 不是 RFC3339: %q", fileEntry.ModTime)
	}
	if sub, ok := byName["sub"]; !ok || !sub.IsDir {
		t.Fatalf("sub 元信息异常: %+v", sub)
	}
	if link, ok := byName["link"]; !ok || !link.IsSymlink || link.IsDir {
		t.Fatalf("link 元信息异常: %+v", link)
	}
}

// TestReadText 真实读取 UTF-8 文本，size 按字节计算。
func TestReadText(t *testing.T) {
	root := t.TempDir()
	content := "hello 世界"
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	var got ReadResult
	if perr := mustCall(t, "file.read", `{"path":"a.txt"}`, root, &got); perr != nil {
		t.Fatalf("file.read 失败: %v", perr)
	}
	if got.Content == nil || *got.Content != content {
		t.Fatalf("content=%v，期望 %q", got.Content, content)
	}
	if got.Size != int64(len(content)) || got.IsBinary || got.Truncated {
		t.Fatalf("read 结果异常: %+v", got)
	}
}

// TestReadBinary 含 NUL 判为二进制，返回 is_binary 且不回 content。
func TestReadBinary(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "bin"), []byte{0x00, 0x01, 0xff}, 0o644); err != nil {
		t.Fatal(err)
	}
	var got ReadResult
	if perr := mustCall(t, "file.read", `{"path":"bin"}`, root, &got); perr != nil {
		t.Fatalf("file.read 失败: %v", perr)
	}
	if !got.IsBinary || got.Content != nil || got.Size != 3 {
		t.Fatalf("二进制结果异常: %+v", got)
	}
}

// TestReadOverLimit 超过默认 1 MiB 返回 E_INVALID 并提示 file.stat。
func TestReadOverLimit(t *testing.T) {
	root := t.TempDir()
	f, err := os.Create(filepath.Join(root, "big"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(DefaultReadLimit + 1); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	_, perr := Call("file.read", json.RawMessage(`{"path":"big"}`), root, nil)
	if perr == nil || perr.Code != pluginapi.E_INVALID {
		t.Fatalf("超限错误=%v，期望 E_INVALID", perr)
	}
	if !strings.Contains(perr.Message, "file.stat") {
		t.Fatalf("超限提示未指引用 file.stat: %q", perr.Message)
	}
	if strings.Contains(perr.Message, root) {
		t.Fatalf("错误信息泄露宿主路径: %q", perr.Message)
	}
}

// TestReadMaxBytes 显式 max_bytes 小于默认值会拒绝，调大到 2 MiB 可读取 1 MiB+ 文件。
func TestReadMaxBytes(t *testing.T) {
	root := t.TempDir()
	content := bytes.Repeat([]byte{'a'}, int(DefaultReadLimit)+10)
	if err := os.WriteFile(filepath.Join(root, "big"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, perr := Call("file.read", json.RawMessage(`{"path":"big","max_bytes":1024}`), root, nil); perr == nil || perr.Code != pluginapi.E_INVALID {
		t.Fatalf("max_bytes=1024 错误=%v，期望 E_INVALID", perr)
	}
	var got ReadResult
	if perr := mustCall(t, "file.read", `{"path":"big","max_bytes":2097152}`, root, &got); perr != nil {
		t.Fatalf("max_bytes=2MiB 读取失败: %v", perr)
	}
	if got.Size != int64(len(content)) || got.Content == nil || len(*got.Content) != len(content) {
		t.Fatalf("max_bytes 放宽后结果异常: size=%d", got.Size)
	}
}

// TestReadDirRejected 对目录调用 file.read 返回 E_INVALID 并指引用 file.list。
func TestReadDirRejected(t *testing.T) {
	root := t.TempDir()
	_, perr := Call("file.read", json.RawMessage(`{"path":"."}`), root, nil)
	if perr == nil || perr.Code != pluginapi.E_INVALID || !strings.Contains(perr.Message, "file.list") {
		t.Fatalf("目录读取错误=%v", perr)
	}
}
