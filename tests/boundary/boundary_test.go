// Package boundary 覆盖 file 插件 production 判据①的四类边界用例：
// 异常输入、并发、权限不足、资源耗尽（PLUGIN-CATALOG §6.1 升级判据①）。
//
// 与 tests/fault 的分工：fault 注入的是「执行面返回某类故障错误」（进程内故障类别），
// 本包验证真实环境下的边界——真实文件系统的 EACCES、内核 RLIMIT_FSIZE 的 EFBIG，
// 以及多 goroutine 真并发下的数据完整性（断言依赖 -race 与内容完整性，不依赖实现细节）。
//
// 用例全部走插件公开方法面（fileop.Call + 真实文件系统），不做测试替身，
// 因此随 `make check` 的 `go test ./...` 自动进 CI（.github/workflows/ci.yml）。
package boundary

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pengtang1024/kore-plugin-file/internal/fileop"
	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// writeParams 是 file.write 的显式入参形状。
// 测试侧独立声明（不依赖内部未导出类型），保证断言绑定的是方法面契约而不是实现。
type writeParams struct {
	Path            string `json:"path"`
	Content         string `json:"content"`
	CreateIfMissing bool   `json:"create_if_missing"`
}

// writeJSON 生成 file.write 的参数 JSON。
func writeJSON(path, content string) json.RawMessage {
	data, err := json.Marshal(writeParams{Path: path, Content: content})
	if err != nil {
		// 显式结构体的序列化不会失败；真失败说明测试自身有 bug，直接 panic 暴露。
		panic("file.write 参数序列化失败: " + err.Error())
	}
	return data
}

// call 走插件公开方法面：fileop.Call + 真实文件系统（exec=nil）。
func call(t *testing.T, root, method string, params json.RawMessage) (json.RawMessage, *pluginapi.Error) {
	t.Helper()
	return fileop.Call(method, params, root, nil)
}

// mustCall 断言调用成功并返回非空结果。
func mustCall(t *testing.T, root, method string, params json.RawMessage) json.RawMessage {
	t.Helper()
	result, perr := call(t, root, method, params)
	if perr != nil {
		t.Fatalf("%s 意外失败: %s: %s", method, perr.Code, perr.Message)
	}
	if len(result) == 0 {
		t.Fatalf("%s 返回空结果", method)
	}
	return result
}

// successProblem 检查调用是否成功（perr 为 nil），返回空串表示满足。
// 并发 goroutine 里不能用 t.Fatalf，用它把失败原因经 channel 交回测试 goroutine。
func successProblem(perr *pluginapi.Error) string {
	if perr == nil {
		return ""
	}
	return perr.Code + ": " + perr.Message
}

// errorProblem 检查错误是否满足期望，返回空串表示满足。
// 拆成「返回问题描述」而不是直接 t.Fatalf，是为了让并发 goroutine 能安全收集问题
// （testing.T 的 Fatalf 只能在测试 goroutine 调用）。
func errorProblem(result json.RawMessage, perr *pluginapi.Error, wantCode, wantHint string) string {
	switch {
	case perr == nil:
		return "期望 " + wantCode + " 错误，实际成功（结果=" + string(result) + "）"
	case result != nil:
		return "出错时不应返回结果: " + string(result)
	case perr.Code != wantCode:
		return "错误码=" + perr.Code + "，期望 " + wantCode + "（message=" + perr.Message + "）"
	case wantHint != "" && !strings.Contains(perr.Message, wantHint):
		return "错误信息 " + perr.Message + " 未含关键词 " + wantHint
	}
	return ""
}

// requireError 在主测试 goroutine 中断言错误契约：错误码 + 消息关键词 + 不带结果。
func requireError(t *testing.T, result json.RawMessage, perr *pluginapi.Error, wantCode, wantHint string) {
	t.Helper()
	if problem := errorProblem(result, perr, wantCode, wantHint); problem != "" {
		t.Fatal(problem)
	}
}
