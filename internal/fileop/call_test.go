package fileop

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// TestBusinessVerb 方法名归一化：file.<verb> 与裸 <verb> 都接受，六方法齐备。
func TestBusinessVerb(t *testing.T) {
	cases := map[string]string{
		"file.list": "list", "file.read": "read", "file.write": "write",
		"file.delete": "delete", "file.mkdir": "mkdir", "file.stat": "stat",
		"list": "list", "read": "read", "write": "write",
	}
	for method, want := range cases {
		got, ok := businessVerb(method)
		if !ok || got != want {
			t.Fatalf("businessVerb(%q)=(%q,%v)，期望 (%q,true)", method, got, ok, want)
		}
	}
	for _, bad := range []string{"file.exec", "file", "file.", "chmod"} {
		if _, ok := businessVerb(bad); ok {
			t.Fatalf("businessVerb(%q) 不应被接受", bad)
		}
	}
}

// TestCallUnknownMethod 未知方法返回 E_NOTFOUND。
func TestCallUnknownMethod(t *testing.T) {
	_, perr := Call("file.exec", json.RawMessage(`{}`), t.TempDir(), nil)
	if perr == nil || perr.Code != pluginapi.E_NOTFOUND {
		t.Fatalf("未知方法错误=%v，期望 E_NOTFOUND", perr)
	}
}

// TestCallParamValidation 非法参数形状、非法 mode、超限内容与读取上限均返回 E_INVALID。
func TestCallParamValidation(t *testing.T) {
	root := t.TempDir()
	bad := []struct {
		method string
		params string
	}{
		{"file.list", `[]`},
		{"file.read", `not-json`},
		{"file.read", `{"path":"a.txt","max_bytes":-1}`},
		{"file.read", `{"path":"a.txt","max_bytes":` + "8388609" + `}`},
		{"file.write", `{"path":"a.txt","content":"` + strings.Repeat("x", MaxWriteBytes+1) + `"}`},
		{"file.mkdir", `{"path":"d","mode":"0999"}`},
		{"file.mkdir", `{"path":"d","mode":"rwxr-xr-x"}`},
		{"file.delete", `{"path":""}`},
	}
	for _, c := range bad {
		_, perr := Call(c.method, json.RawMessage(c.params), root, nil)
		if perr == nil || perr.Code != pluginapi.E_INVALID {
			t.Fatalf("Call(%s,%s) 错误=%v，期望 E_INVALID", c.method, truncate(c.params), perr)
		}
	}
}

// truncate 截断过长的参数文本，避免测试失败信息刷屏。
func truncate(s string) string {
	if len(s) > 48 {
		return s[:48] + "..."
	}
	return s
}
