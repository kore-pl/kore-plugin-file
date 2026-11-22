package fileop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

func assertInvalid(t *testing.T, method, params, root, outside string) {
	t.Helper()
	_, perr := Call(method, json.RawMessage(params), root, nil)
	if perr == nil || perr.Code != pluginapi.E_INVALID {
		t.Fatalf("%s %s 错误=%v，期望 E_INVALID", method, params, perr)
	}
	if perr.Message == "" {
		t.Fatalf("%s 拒绝时未说明原因", method)
	}
	if (root != "" && strings.Contains(perr.Message, root)) || (outside != "" && strings.Contains(perr.Message, outside)) {
		t.Fatalf("%s 错误信息泄露宿主路径: %q", method, perr.Message)
	}
}

// TestSecurityLexicalReject 穿越、绝对路径、NUL、反斜杠在词法层全部拒绝。
func TestSecurityLexicalReject(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name   string
		method string
		params string
	}{
		{"scan-up", "file.read", `{"path":"../../etc/passwd"}`},
		{"absolute", "file.read", `{"path":"/etc/passwd"}`},
		{"mixed-up", "file.write", `{"path":"a/../../b","content":"x"}`},
		{"list-up", "file.list", `{"path":"../../"}`},
		{"delete-up", "file.delete", `{"path":"../x"}`},
		{"mkdir-up", "file.mkdir", `{"path":"../m"}`},
		{"stat-up", "file.stat", `{"path":"../../etc/shadow"}`},
		{"nul", "file.read", `{"path":"a\u0000b"}`},
		{"backslash", "file.read", `{"path":"a\\b"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { assertInvalid(t, c.method, c.params, root, "") })
	}
}

// TestSecuritySymlinkEscape 指向根目录外的符号链接：六个方法全部拒绝。
func TestSecuritySymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(root, "file-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "dir-link")); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		method string
		params string
	}{
		{"read-link", "file.read", `{"path":"file-link"}`},
		{"stat-link", "file.stat", `{"path":"file-link"}`},
		{"list-link", "file.list", `{"path":"file-link"}`},
		{"delete-link", "file.delete", `{"path":"file-link"}`},
		{"write-through", "file.write", `{"path":"dir-link/pwned","content":"x","create_if_missing":true}`},
		{"mkdir-through", "file.mkdir", `{"path":"dir-link/newdir"}`},
		{"read-through", "file.read", `{"path":"dir-link/secret"}`},
		{"list-through", "file.list", `{"path":"dir-link"}`},
		{"stat-through", "file.stat", `{"path":"dir-link"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { assertInvalid(t, c.method, c.params, root, outside) })
	}
}

// TestSecuritySymlinkInsideAllowed 指向根目录内的符号链接是合法路径，可读且删除只删链接。
func TestSecuritySymlinkInsideAllowed(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "a.txt", "inside")
	if err := os.Symlink("a.txt", filepath.Join(root, "inside-link")); err != nil {
		t.Fatal(err)
	}
	var got ReadResult
	if perr := mustCall(t, "file.read", `{"path":"inside-link"}`, root, &got); perr != nil {
		t.Fatalf("根内链接读取失败: %v", perr)
	}
	if got.Content == nil || *got.Content != "inside" {
		t.Fatalf("根内链接内容=%v", got.Content)
	}
	if perr := mustCall(t, "file.delete", `{"path":"inside-link"}`, root, nil); perr != nil {
		t.Fatalf("删除根内链接失败: %v", perr)
	}
	if _, err := os.Lstat(filepath.Join(root, "inside-link")); !os.IsNotExist(err) {
		t.Fatalf("链接仍存在: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "a.txt")); err != nil {
		t.Fatalf("链接目标被误删: %v", err)
	}
}

// TestSecurityRootSymlink 根目录自身是符号链接时，操作仍限制在解析后的真实根内。
func TestSecurityRootSymlink(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "root-link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, link, "a.txt", "ok")
	var got ReadResult
	if perr := mustCall(t, "file.read", `{"path":"a.txt"}`, link, &got); perr != nil {
		t.Fatalf("根为符号链接时读取失败: %v", perr)
	}
	if got.Content == nil || *got.Content != "ok" {
		t.Fatalf("内容=%v", got.Content)
	}
}
