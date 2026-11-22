// Package fault 对 file 插件做四类故障注入，验证方法级错误返回。
// 每个用例向 fileop.Call 的执行面注入一种故障，断言方法返回的结构化错误码。
package fault

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/pengtang1024/kore-plugin-file/internal/fileop"
	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// faultExecutor 按注入类别让每个方法都失败，模拟四类运行时故障。
type faultExecutor struct {
	kind fileop.FaultKind
}

func (f faultExecutor) List(string) ([]fileop.Entry, error) {
	return nil, fileop.NewFaultError(f.kind, "list 执行面故障")
}

func (f faultExecutor) Read(string, int64) (fileop.ReadData, error) {
	return fileop.ReadData{}, fileop.NewFaultError(f.kind, "read 执行面故障")
}

func (f faultExecutor) Write(string, string, bool) (fileop.WriteData, error) {
	return fileop.WriteData{}, fileop.NewFaultError(f.kind, "write 执行面故障")
}

func (f faultExecutor) Delete(string, bool) error {
	return fileop.NewFaultError(f.kind, "delete 执行面故障")
}

func (f faultExecutor) Mkdir(string, fs.FileMode) error {
	return fileop.NewFaultError(f.kind, "mkdir 执行面故障")
}

func (f faultExecutor) Stat(string) (fileop.StatData, error) {
	return fileop.StatData{}, fileop.NewFaultError(f.kind, "stat 执行面故障")
}

func (f faultExecutor) Upload(string, []byte, bool) (fileop.UploadData, error) {
	return fileop.UploadData{}, fileop.NewFaultError(f.kind, "upload 执行面故障")
}

func (f faultExecutor) Download(string, int64) (fileop.DownloadData, error) {
	return fileop.DownloadData{}, fileop.NewFaultError(f.kind, "download 执行面故障")
}

func (f faultExecutor) Extract(fileop.ExtractRequest) (fileop.ExtractData, error) {
	return fileop.ExtractData{}, fileop.NewFaultError(f.kind, "extract 执行面故障")
}

func (f faultExecutor) Compress([]fileop.CompressSource, string, string) (fileop.CompressData, error) {
	return fileop.CompressData{}, fileop.NewFaultError(f.kind, "compress 执行面故障")
}

func (f faultExecutor) Chmod(string, fs.FileMode, bool) (fileop.ChmodData, error) {
	return fileop.ChmodData{}, fileop.NewFaultError(f.kind, "chmod 执行面故障")
}

func (f faultExecutor) Move(string, string, bool) (fileop.MoveData, error) {
	return fileop.MoveData{}, fileop.NewFaultError(f.kind, "move 执行面故障")
}

// methods 是故障注入覆盖的方法及其合法参数；顺序保证 sanityNoFault 的真实执行自洽。
var methods = []struct {
	name   string
	method string
	params string
}{
	{"list", "file.list", `{"path":"docs"}`},
	{"read", "file.read", `{"path":"docs/a.txt"}`},
	{"write", "file.write", `{"path":"docs/a.txt","content":"hello"}`},
	{"upload", "file.upload", `{"path":"docs","filename":"up.txt","data_base64":"aGVsbG8="}`},
	{"download", "file.download", `{"path":"docs/a.txt"}`},
	{"compress", "file.compress", `{"paths":["docs"],"archive_path":"out.zip"}`},
	{"chmod", "file.chmod", `{"path":"docs/a.txt","mode":"0600"}`},
	{"move", "file.move", `{"src":"docs/a.txt","dst":"docs/b.txt"}`},
}

// prepareRoot 造一个真实临时根：路径解析必须通过，故障只能来自注入的执行面。
func prepareRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("造 docs 目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "a.txt"), []byte("seed"), 0o644); err != nil {
		t.Fatalf("造种子文件失败: %v", err)
	}
	return root
}

// assertFault 对每个方法注入故障，断言返回 nil 结果 + 预期的协议错误码与类别关键词。
func assertFault(t *testing.T, kind fileop.FaultKind, wantCode, wantHint string) {
	t.Helper()
	root := prepareRoot(t)
	exec := faultExecutor{kind: kind}
	for _, m := range methods {
		t.Run(m.name, func(t *testing.T) {
			result, perr := fileop.Call(m.method, json.RawMessage(m.params), root, exec)
			if perr == nil {
				t.Fatalf("%s 注入 %s 后未返回错误（结果=%s）", m.method, kind, result)
			}
			if result != nil {
				t.Fatalf("%s 出错时不应带结果: %s", m.method, result)
			}
			if perr.Code != wantCode {
				t.Fatalf("%s 错误码=%q，期望 %q（message=%q）", m.method, perr.Code, wantCode, perr.Message)
			}
			if !contains(perr.Message, wantHint) {
				t.Fatalf("%s 错误信息 %q 未含类别关键词 %q", m.method, perr.Message, wantHint)
			}
		})
	}
}

// contains 是 strings.Contains 的本地封装，避免每个 fault 文件重复 import strings。
func contains(s, sub string) bool {
	if len(sub) > len(s) {
		return false
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// sanityNoFault 验证无故障时三方法正常返回，证明故障断言不是"永远失败"的假通过。
func sanityNoFault(t *testing.T) {
	t.Helper()
	root := prepareRoot(t)
	for _, m := range methods {
		result, perr := fileop.Call(m.method, json.RawMessage(m.params), root, nil)
		if perr != nil {
			t.Fatalf("无故障时 %s 意外返回错误: %v", m.method, perr)
		}
		if len(result) == 0 {
			t.Fatalf("无故障时 %s 返回空结果", m.method)
		}
	}
}

// verifyErrorContract 断言错误体是标准协议结构（含 code 字段）。
func verifyErrorContract(t *testing.T, perr *pluginapi.Error) {
	t.Helper()
	if perr == nil {
		t.Fatal("错误体为 nil")
	}
	if perr.Code == "" || perr.Message == "" {
		t.Fatalf("错误体字段不完整: %+v", perr)
	}
}
