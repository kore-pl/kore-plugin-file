package fileop

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// TestBusinessVerb 方法名归一化：file.<verb> 与裸 <verb> 都接受。
func TestBusinessVerb(t *testing.T) {
	cases := map[string]string{
		"file.list": "list", "file.read": "read", "file.write": "write",
		"list": "list", "read": "read", "write": "write",
	}
	for method, want := range cases {
		got, ok := businessVerb(method)
		if !ok || got != want {
			t.Fatalf("businessVerb(%q)=(%q,%v)，期望 (%q,true)", method, got, ok, want)
		}
	}
	if _, ok := businessVerb("file.exec"); ok {
		t.Fatal("file.exec 不应被接受")
	}
}

// TestCallResults 三方法正常路径返回结构化结果。
func TestCallResults(t *testing.T) {
	list, perr := Call("file.list", json.RawMessage(`{"path":"docs"}`), root, nil)
	if perr != nil || !strings.Contains(string(list), `"count":0`) {
		t.Fatalf("file.list 结果=%s err=%v", list, perr)
	}
	read, perr := Call("file.read", json.RawMessage(`{"path":"a.txt"}`), root, nil)
	if perr != nil || !strings.Contains(string(read), `"encoding":"utf-8"`) {
		t.Fatalf("file.read 结果=%s err=%v", read, perr)
	}
	write, perr := Call("file.write", json.RawMessage(`{"path":"a.txt","content":"hi"}`), root, nil)
	if perr != nil || !strings.Contains(string(write), `"bytes_written":2`) {
		t.Fatalf("file.write 结果=%s err=%v", write, perr)
	}
}

// TestCallParamValidation 非法参数形状、路径穿越、超长内容均返回 E_INVALID。
func TestCallParamValidation(t *testing.T) {
	bad := []struct {
		method string
		params string
	}{
		{"file.list", `[]`},
		{"file.read", `not-json`},
		{"file.read", `{"path":"../../etc"}`},
		{"file.write", `{"path":"a.txt","content":"` + strings.Repeat("x", maxContent+1) + `"}`},
	}
	for _, c := range bad {
		_, perr := Call(c.method, json.RawMessage(c.params), root, nil)
		if perr == nil || perr.Code != pluginapi.E_INVALID {
			t.Fatalf("Call(%s,%s) 错误=%v，期望 E_INVALID", c.method, c.params[:min(24, len(c.params))], perr)
		}
	}
}

// TestCallUnknownMethod 未知方法返回 E_NOTFOUND。
func TestCallUnknownMethod(t *testing.T) {
	_, perr := Call("file.exec", json.RawMessage(`{}`), root, nil)
	if perr == nil || perr.Code != pluginapi.E_NOTFOUND {
		t.Fatalf("未知方法错误=%v，期望 E_NOTFOUND", perr)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
