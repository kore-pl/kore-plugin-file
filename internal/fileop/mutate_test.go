package fileop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// mustWrite 用 file.write 造一个真实文件，供 delete / stat 用例复用。
func mustWrite(t *testing.T, root, rel, content string) {
	t.Helper()
	params := `{"path":"` + rel + `","content":"` + content + `"}`
	if perr := mustCall(t, "file.write", params, root, nil); perr != nil {
		t.Fatalf("准备文件 %s 失败: %v", rel, perr)
	}
}

// TestDeleteFile 删除文件后目标消失，重复删除返回 E_NOTFOUND。
func TestDeleteFile(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "a.txt", "x")
	var got DeleteResult
	if perr := mustCall(t, "file.delete", `{"path":"a.txt"}`, root, &got); perr != nil {
		t.Fatalf("删除文件失败: %v", perr)
	}
	if !got.Deleted || got.Path != "a.txt" {
		t.Fatalf("删除结果异常: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(root, "a.txt")); !os.IsNotExist(err) {
		t.Fatalf("文件仍存在: %v", err)
	}
	if _, perr := Call("file.delete", json.RawMessage(`{"path":"a.txt"}`), root, nil); perr == nil || perr.Code != pluginapi.E_NOTFOUND {
		t.Fatalf("重复删除错误=%v，期望 E_NOTFOUND", perr)
	}
}

// TestDeleteDir 空目录直接删；非空目录 recursive=false 回 E_CONFLICT，true 才删除。
func TestDeleteDir(t *testing.T) {
	root := t.TempDir()
	if perr := mustCall(t, "file.mkdir", `{"path":"empty"}`, root, nil); perr != nil {
		t.Fatal(perr)
	}
	if perr := mustCall(t, "file.delete", `{"path":"empty"}`, root, nil); perr != nil {
		t.Fatalf("删除空目录失败: %v", perr)
	}
	if perr := mustCall(t, "file.mkdir", `{"path":"full"}`, root, nil); perr != nil {
		t.Fatal(perr)
	}
	mustWrite(t, root, "full/a.txt", "x")
	_, perr := Call("file.delete", json.RawMessage(`{"path":"full"}`), root, nil)
	if perr == nil || perr.Code != pluginapi.E_CONFLICT || !strings.Contains(perr.Message, "recursive=true") {
		t.Fatalf("非空目录错误=%v，期望 E_CONFLICT", perr)
	}
	if _, err := os.Stat(filepath.Join(root, "full", "a.txt")); err != nil {
		t.Fatalf("非空目录被误删: %v", err)
	}
	if perr := mustCall(t, "file.delete", `{"path":"full","recursive":true}`, root, nil); perr != nil {
		t.Fatalf("递归删除失败: %v", perr)
	}
	if _, err := os.Stat(filepath.Join(root, "full")); !os.IsNotExist(err) {
		t.Fatalf("目录仍存在: %v", err)
	}
}

// TestDeleteRoot 配置根目录本身禁止删除。
func TestDeleteRoot(t *testing.T) {
	root := t.TempDir()
	_, perr := Call("file.delete", json.RawMessage(`{"path":"."}`), root, nil)
	if perr == nil || perr.Code != pluginapi.E_INVALID {
		t.Fatalf("删除根目录错误=%v，期望 E_INVALID", perr)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("根目录被误删: %v", err)
	}
}

// TestMkdir 新建目录默认 0755、显式 mode 生效、已存在回 E_CONFLICT、父目录缺失回 E_NOTFOUND。
func TestMkdir(t *testing.T) {
	root := t.TempDir()
	var got MkdirResult
	if perr := mustCall(t, "file.mkdir", `{"path":"d","mode":"0700"}`, root, &got); perr != nil {
		t.Fatalf("建目录失败: %v", perr)
	}
	if !got.Created || got.Mode != "0700" {
		t.Fatalf("建目录结果异常: %+v", got)
	}
	info, err := os.Stat(filepath.Join(root, "d"))
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("目录元信息=%v err=%v", info, err)
	}
	if _, perr := Call("file.mkdir", json.RawMessage(`{"path":"d"}`), root, nil); perr == nil || perr.Code != pluginapi.E_CONFLICT {
		t.Fatalf("重复建目录错误=%v，期望 E_CONFLICT", perr)
	}
	if _, perr := Call("file.mkdir", json.RawMessage(`{"path":"missing/sub"}`), root, nil); perr == nil || perr.Code != pluginapi.E_NOTFOUND {
		t.Fatalf("父目录缺失错误=%v，期望 E_NOTFOUND", perr)
	}
}

// TestStat 文件与目录的 size/mode/is_dir/is_symlink 断言。
func TestStat(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "a.txt", "data")
	path := filepath.Join(root, "a.txt")
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	var file StatResult
	if perr := mustCall(t, "file.stat", `{"path":"a.txt"}`, root, &file); perr != nil {
		t.Fatalf("stat 文件失败: %v", perr)
	}
	if file.Size != 4 || file.Mode != "0640" || file.IsDir || file.IsSymlink {
		t.Fatalf("文件 stat 异常: %+v", file)
	}
	var dir StatResult
	if perr := mustCall(t, "file.stat", `{"path":"."}`, root, &dir); perr != nil {
		t.Fatalf("stat 目录失败: %v", perr)
	}
	if !dir.IsDir || dir.Path != "." {
		t.Fatalf("目录 stat 异常: %+v", dir)
	}
	if _, perr := Call("file.stat", json.RawMessage(`{"path":"nope"}`), root, nil); perr == nil || perr.Code != pluginapi.E_NOTFOUND {
		t.Fatalf("缺失路径错误=%v，期望 E_NOTFOUND", perr)
	}
}

// TestStatSymlinkInside 指向根目录内的符号链接可 stat，is_symlink=true 且元信息来自目标。
func TestStatSymlinkInside(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "a.txt", "data")
	if err := os.Symlink("a.txt", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	var got StatResult
	if perr := mustCall(t, "file.stat", `{"path":"link"}`, root, &got); perr != nil {
		t.Fatalf("stat 链接失败: %v", perr)
	}
	if !got.IsSymlink || got.IsDir || got.Size != 4 {
		t.Fatalf("链接 stat 异常: %+v", got)
	}
}
