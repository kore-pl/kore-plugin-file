package fault

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/pengtang1024/kore-plugin-file/internal/fileop"
)

// faultCases 是四类故障到协议错误码的期望映射（与 kill/disk/timeout/cgroup 测试同源）。
var faultCases = []struct {
	name string
	kind fileop.FaultKind
	code string
	hint string
}{
	{"kill", fileop.FaultKill, "E_UPSTREAM", "信号 9"},
	{"timeout", fileop.FaultTimeout, "E_UPSTREAM", "超时"},
	{"disk", fileop.FaultDisk, "E_INTERNAL", "磁盘满"},
	{"cgroup", fileop.FaultCgroup, "E_UNSUPPORTED", "cgroup"},
}

// TestBatchPartialFailureFault 批量删除在故障下逐项返回映射后的错误码，整体仍是成功帧
// （部分失败不吞错、也不整体失败）。
func TestBatchPartialFailureFault(t *testing.T) {
	for _, tc := range faultCases {
		t.Run(tc.name, func(t *testing.T) {
			root := prepareRoot(t)
			params := `{"op":"delete","items":[{"path":"docs/batch1.txt","permanent":true},{"path":"docs/batch2.txt","permanent":true}]}`
			result, perr := fileop.Call("file.batch", json.RawMessage(params), root, faultExecutor{kind: tc.kind})
			if perr != nil {
				t.Fatalf("批量部分失败不应返回整体错误: %v", perr)
			}
			var got fileop.BatchResult
			if err := json.Unmarshal(result, &got); err != nil {
				t.Fatalf("批量结果解码失败: %v", err)
			}
			if got.Total != 2 || got.Failed != 2 || got.Succeeded != 0 {
				t.Fatalf("批量统计异常: %+v", got)
			}
			for _, item := range got.Items {
				if item.OK || item.Error == nil {
					t.Fatalf("条目应失败: %+v", item)
				}
				if item.Error.Code != tc.code || !contains(item.Error.Message, tc.hint) {
					t.Fatalf("条目错误=%+v，期望 %s 含 %q", item.Error, tc.code, tc.hint)
				}
			}
		})
	}
}

// TestTrashFaults 回收站三个方法在故障下都返回统一映射的协议错误码。
func TestTrashFaults(t *testing.T) {
	for _, tc := range faultCases {
		t.Run(tc.name, func(t *testing.T) {
			root := prepareRoot(t)
			// 用真实执行面先造一条回收站条目，避免注入面影响准备阶段。
			var deleted fileop.DeleteResult
			raw, perr := fileop.Call("file.delete", json.RawMessage(`{"path":"docs/a.txt"}`), root, nil)
			if perr != nil {
				t.Fatalf("准备回收站条目失败: %v", perr)
			}
			if err := json.Unmarshal(raw, &deleted); err != nil || deleted.TrashID == "" {
				t.Fatalf("删除结果异常: %s err=%v", raw, err)
			}
			cases := []struct{ name, method, params string }{
				{"trash.list", "file.trash.list", `{}`},
				{"trash.restore", "file.trash.restore", fmt.Sprintf(`{"id":%q}`, deleted.TrashID)},
				{"trash.purge", "file.trash.purge", fmt.Sprintf(`{"id":%q}`, deleted.TrashID)},
			}
			exec := faultExecutor{kind: tc.kind}
			for _, c := range cases {
				result, callErr := fileop.Call(c.method, json.RawMessage(c.params), root, exec)
				if callErr == nil {
					t.Fatalf("%s 注入 %s 后未返回错误（结果=%s）", c.method, tc.kind, result)
				}
				if result != nil {
					t.Fatalf("%s 出错时不应带结果", c.method)
				}
				if callErr.Code != tc.code || !contains(callErr.Message, tc.hint) {
					t.Fatalf("%s 错误=%+v，期望 %s 含 %q", c.method, callErr, tc.code, tc.hint)
				}
				verifyErrorContract(t, callErr)
			}
		})
	}
}
