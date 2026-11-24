package boundary

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// requireNonRoot：权限位对 root 无效（DAC_OVERRIDE），root 下断言 EACCES 是假测试，
// 因此显式 Skip 并说明原因，而不是在 root 下让用例静默变成无效断言。
func requireNonRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("以 root 运行：DAC_OVERRIDE 会绕过权限位，本组用例只在非 root 下成立")
	}
}

// TestPermissionUnreadableFile 真正不可读的文件（mode 0000）读取必须失败：
// 执行面 os.Open 返回 EACCES，方法面按现有契约映射为 E_INTERNAL + 「权限不足」。
func TestPermissionUnreadableFile(t *testing.T) {
	t.Parallel()
	requireNonRoot(t)

	root := t.TempDir()
	target := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatalf("造文件失败: %v", err)
	}
	if err := os.Chmod(target, 0o000); err != nil {
		t.Fatalf("chmod 000 失败: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(target, 0o600) })

	result, perr := call(t, root, "file.read", json.RawMessage(`{"path":"secret.txt"}`))
	requireError(t, result, perr, pluginapi.E_INTERNAL, "权限不足")
}

// TestPermissionUnlistableDir 真正不可读的目录（mode 0000）列目录必须失败。
func TestPermissionUnlistableDir(t *testing.T) {
	t.Parallel()
	requireNonRoot(t)

	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	if err := os.Mkdir(locked, 0o750); err != nil {
		t.Fatalf("造目录失败: %v", err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatalf("chmod 000 失败: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o750) })

	result, perr := call(t, root, "file.list", json.RawMessage(`{"path":"locked"}`))
	requireError(t, result, perr, pluginapi.E_INTERNAL, "权限不足")
}

// TestPermissionWriteIntoReadOnlyDir 只读目录（0555）里写新文件必须失败：
// 断言 E_INTERNAL + 「权限不足」，并确认失败后不落地目标文件、不残留 .kore-file-*.tmp。
func TestPermissionWriteIntoReadOnlyDir(t *testing.T) {
	t.Parallel()
	requireNonRoot(t)

	root := t.TempDir()
	readonly := filepath.Join(root, "readonly")
	if err := os.Mkdir(readonly, 0o750); err != nil {
		t.Fatalf("造只读目录失败: %v", err)
	}
	if err := os.Chmod(readonly, 0o555); err != nil {
		t.Fatalf("chmod 0555 失败: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(readonly, 0o750) })

	result, perr := call(t, root, "file.write", writeJSON("readonly/new.txt", "blocked"))
	requireError(t, result, perr, pluginapi.E_INTERNAL, "权限不足")

	if _, err := os.Stat(filepath.Join(readonly, "new.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("权限不足时目标文件不应落地: stat err=%v", err)
	}
	leftovers, err := filepath.Glob(filepath.Join(readonly, ".kore-file-*.tmp"))
	if err != nil {
		t.Fatalf("扫描临时文件失败: %v", err)
	}
	if len(leftovers) != 0 {
		t.Fatalf("权限不足时残留临时文件: %v", leftovers)
	}
}

// TestPermissionDeleteInReadOnlyDir 只读目录（0555）里直接删除文件必须失败，
// 且失败后文件仍在（不能出现「报错但已删」）。
func TestPermissionDeleteInReadOnlyDir(t *testing.T) {
	t.Parallel()
	requireNonRoot(t)

	root := t.TempDir()
	readonly := filepath.Join(root, "readonly")
	if err := os.Mkdir(readonly, 0o750); err != nil {
		t.Fatalf("造只读目录失败: %v", err)
	}
	victim := filepath.Join(readonly, "victim.txt")
	if err := os.WriteFile(victim, []byte("keep"), 0o644); err != nil {
		t.Fatalf("造待删文件失败: %v", err)
	}
	if err := os.Chmod(readonly, 0o555); err != nil {
		t.Fatalf("chmod 0555 失败: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(readonly, 0o750) })

	result, perr := call(t, root, "file.delete", json.RawMessage(`{"path":"readonly/victim.txt","permanent":true}`))
	requireError(t, result, perr, pluginapi.E_INTERNAL, "权限不足")

	if _, err := os.Stat(victim); err != nil {
		t.Fatalf("权限不足时文件不应被删除: %v", err)
	}
}

// TestPermissionRootOnlyPathUnreachable root-only 路径在词法面就被拒绝（E_INVALID），
// 根本到不了执行面：这保证「权限不足」只来自真实的 EACCES，而不是路径越界被误分类。
func TestPermissionRootOnlyPathUnreachable(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cases := []struct {
		name   string
		method string
		params string
	}{
		{"root私钥", "file.read", `{"path":"/root/.ssh/id_ed25519"}`},
		{"上层穿越", "file.read", `{"path":"../../etc/shadow"}`},
		{"回收站越权", "file.list", `{"path":".trash/entries"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, perr := call(t, root, tc.method, json.RawMessage(tc.params))
			requireError(t, result, perr, pluginapi.E_INVALID, "")
		})
	}
}
