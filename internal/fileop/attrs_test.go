package fileop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// TestChmodFile 文件 0644 → 0600 生效，重复设置同一 mode 时 changed=0。
func TestChmodFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "a.txt")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var got ChmodResult
	if perr := mustCall(t, "file.chmod", `{"path":"a.txt","mode":"0600"}`, root, &got); perr != nil {
		t.Fatalf("file.chmod 失败: %v", perr)
	}
	if got.Mode != "0600" || got.Changed != 1 {
		t.Fatalf("chmod 结果异常: %+v", got)
	}
	info, err := os.Stat(target)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("落盘权限=%v err=%v，期望 0600", info.Mode().Perm(), err)
	}
	var again ChmodResult
	if perr := mustCall(t, "file.chmod", `{"path":"a.txt","mode":"0600"}`, root, &again); perr != nil || again.Changed != 0 {
		t.Fatalf("重复 chmod 结果=%+v err=%v，期望 changed=0", again, perr)
	}
}

// TestChmodRecursive 目录树逐项应用同一 mode，目录保留执行位。
func TestChmodRecursive(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "d", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "d", "sub", "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var got ChmodResult
	if perr := mustCall(t, "file.chmod", `{"path":"d","mode":"0750","recursive":true}`, root, &got); perr != nil {
		t.Fatalf("递归 chmod 失败: %v", perr)
	}
	if got.Changed != 3 {
		t.Fatalf("changed=%d，期望 3（d + sub + f.txt）", got.Changed)
	}
	for _, rel := range []string{"d", "d/sub", "d/sub/f.txt"} {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil || info.Mode().Perm() != 0o750 {
			t.Fatalf("%s 权限=%v err=%v，期望 0750", rel, info.Mode().Perm(), err)
		}
	}
}

// TestChmodDirWithoutExec 目录去掉执行位必须报 E_INVALID，且不改任何权限。
func TestChmodDirWithoutExec(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "d", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "d", "sub", "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, params := range []string{
		`{"path":"d","mode":"0644"}`,
		`{"path":"d","mode":"0600","recursive":true}`,
	} {
		_, perr := Call("file.chmod", jsonRaw(params), root, nil)
		if perr == nil || perr.Code != pluginapi.E_INVALID || !strings.Contains(perr.Message, "执行位") {
			t.Fatalf("%s 错误=%v，期望 E_INVALID + 执行位提示", params, perr)
		}
	}
	info, err := os.Stat(filepath.Join(root, "d"))
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("拒绝后目录权限=%v err=%v，期望保持 0755", info.Mode().Perm(), err)
	}
	if info, err := os.Stat(filepath.Join(root, "d", "sub", "f.txt")); err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("拒绝后文件权限=%v err=%v，期望保持 0644", info.Mode().Perm(), err)
	}
}

// TestChmodParamValidation mode 必须是合法八进制，根目录禁止改权限。
func TestChmodParamValidation(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []string{
		`{"path":"a.txt","mode":""}`,
		`{"path":"a.txt","mode":"0999"}`,
		`{"path":"a.txt","mode":"rwxr-xr-x"}`,
		`{"path":".","mode":"0755"}`,
	}
	for _, params := range cases {
		_, perr := Call("file.chmod", jsonRaw(params), root, nil)
		if perr == nil || perr.Code != pluginapi.E_INVALID {
			t.Fatalf("%s 错误=%v，期望 E_INVALID", params, perr)
		}
	}
}
