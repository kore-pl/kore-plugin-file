package boundary

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// TestAbnormalInputContract 复用现有测试里的代表性异常输入（来自
// internal/fileop/call_test.go 的 TestCallParamValidation 与 path_test.go 的
// TestValidatePathReject），在公开方法面 + 真实文件系统上重新钉住契约：
// 非法 JSON、非对象参数、绝对路径、上层穿越、NUL 字节、未知方法、回收站保留路径、
// max_bytes 越界、读目录。额外断言错误信息不泄露宿主绝对路径。
func TestAbnormalInputContract(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("造 docs 目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "a.txt"), []byte("seed"), 0o644); err != nil {
		t.Fatalf("造种子文件失败: %v", err)
	}

	cases := []struct {
		name     string
		method   string
		params   string
		wantCode string
		wantHint string
	}{
		{"非法JSON", "file.read", `not-json`, pluginapi.E_INVALID, "params"},
		{"非对象参数", "file.list", `[]`, pluginapi.E_INVALID, "params"},
		{"绝对路径", "file.read", `{"path":"/etc/passwd"}`, pluginapi.E_INVALID, "相对路径"},
		{"上层穿越", "file.read", `{"path":"../etc/shadow"}`, pluginapi.E_INVALID, ".."},
		{"NUL字节", "file.read", `{"path":"docs/\u0000a"}`, pluginapi.E_INVALID, "NUL"},
		{"未知方法", "file.exec", `{}`, pluginapi.E_NOTFOUND, "未知方法"},
		{"回收站保留路径", "file.list", `{"path":".trash"}`, pluginapi.E_INVALID, ".trash"},
		{"max_bytes越界", "file.read", `{"path":"docs/a.txt","max_bytes":8388609}`, pluginapi.E_INVALID, "max_bytes"},
		{"读目录", "file.read", `{"path":"docs"}`, pluginapi.E_INVALID, "目录"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, perr := call(t, root, tc.method, json.RawMessage(tc.params))
			requireError(t, result, perr, tc.wantCode, tc.wantHint)
			if perr == nil {
				return // requireError 已 t.Fatal；此行只为让静态检查看到分支
			}
			if strings.Contains(perr.Message, root) {
				t.Fatalf("错误信息泄露宿主绝对路径 %q: %q", root, perr.Message)
			}
		})
	}
}
