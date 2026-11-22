package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// dispatchCall 把方法调用打到 dispatch 并返回响应。
func dispatchCall(t *testing.T, cfg config, method, params string) *pluginapi.Response {
	t.Helper()
	req := &pluginapi.Request{
		Type: pluginapi.TypeRequest, ID: "id", ReqID: "r",
		Method: method, Params: json.RawMessage(params),
	}
	resp, shutdown := dispatch(req, cfg, testLogger())
	if shutdown {
		t.Fatalf("%s 不应触发 shutdown", method)
	}
	return resp
}

// TestDispatchRealOps 六个 file 方法经 dispatch 走真实文件系统：建目录 → 写 → 列 → 读 → stat → 删。
func TestDispatchRealOps(t *testing.T) {
	root := t.TempDir()
	cfg := testConfig(root)

	check := func(method, params string, want ...string) string {
		t.Helper()
		resp := dispatchCall(t, cfg, method, params)
		if resp.Error != nil {
			t.Fatalf("%s 意外错误: %+v", method, resp.Error)
		}
		got := string(resp.Result)
		for _, w := range want {
			if !strings.Contains(got, w) {
				t.Fatalf("%s 结果=%s，未含 %q", method, got, w)
			}
		}
		return got
	}

	check("file.mkdir", `{"path":"docs"}`, `"created":true`, `"mode":"0755"`)
	check("file.write", `{"path":"docs/a.txt","content":"hello"}`, `"created":true`, `"size":5`)
	check("file.list", `{"path":"docs"}`, `"count":1`, `"name":"a.txt"`, `"is_dir":false`)
	check("file.read", `{"path":"docs/a.txt"}`, `"content":"hello"`, `"size":5`, `"is_binary":false`)
	check("file.stat", `{"path":"docs/a.txt"}`, `"size":5`, `"is_dir":false`, `"owner":{"uid":`)
	check("file.delete", `{"path":"docs/a.txt"}`, `"deleted":true`)
	check("file.delete", `{"path":"docs","recursive":true}`, `"deleted":true`)
}

// TestDispatchReject 路径穿越、删除根目录与未知方法经 dispatch 返回协议错误。
func TestDispatchReject(t *testing.T) {
	cfg := testConfig(t.TempDir())
	cases := []struct {
		method string
		params string
		code   string
	}{
		{"file.read", `{"path":"../../etc/passwd"}`, pluginapi.E_INVALID},
		{"file.read", `{"path":"/etc/passwd"}`, pluginapi.E_INVALID},
		{"file.write", `{"path":"../escape","content":"x"}`, pluginapi.E_INVALID},
		{"file.delete", `{"path":"."}`, pluginapi.E_INVALID},
		{"file.exec", `{}`, pluginapi.E_NOTFOUND},
	}
	for _, c := range cases {
		resp := dispatchCall(t, cfg, c.method, c.params)
		if resp.Error == nil || resp.Error.Code != c.code {
			t.Fatalf("%s 错误=%v，期望 %q", c.method, resp.Error, c.code)
		}
		if resp.Result != nil {
			t.Fatalf("%s 出错时不应带结果", c.method)
		}
	}
}

// TestPingResult kore.ping 返回 ready 状态。
func TestPingResult(t *testing.T) {
	resp := dispatchCall(t, testConfig(t.TempDir()), "kore.ping", `{}`)
	if !strings.Contains(string(resp.Result), `"status":"ready"`) {
		t.Fatalf("ping 结果=%s", resp.Result)
	}
}
