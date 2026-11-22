package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pengtang1024/kore-plugin-file/internal/fileop"
)

// TestDispatchL2Ops 六个 L2 方法经 dispatch 走真实文件系统，验证方法路由与结果形状。
func TestDispatchL2Ops(t *testing.T) {
	root := t.TempDir()
	cfg := testConfig(root)
	call := func(method, params string) json.RawMessage {
		t.Helper()
		resp := dispatchCall(t, cfg, method, params)
		if resp.Error != nil {
			t.Fatalf("%s 意外错误: %+v", method, resp.Error)
		}
		return resp.Result
	}
	decode := func(method string, raw json.RawMessage, out any) {
		t.Helper()
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("%s 结果解码失败: %v", method, err)
		}
	}
	payload := []byte("hello-l2")
	upload := fmt.Sprintf(`{"path":".","filename":"up.txt","data_base64":%q}`,
		base64.StdEncoding.EncodeToString(payload))
	var up fileop.UploadResult
	decode("file.upload", call("file.upload", upload), &up)
	if up.Size != int64(len(payload)) || up.SHA256 == "" {
		t.Fatalf("upload 结果异常: %+v", up)
	}
	var down fileop.DownloadResult
	decode("file.download", call("file.download", `{"path":"up.txt"}`), &down)
	if data, err := base64.StdEncoding.DecodeString(down.DataBase64); err != nil || string(data) != string(payload) {
		t.Fatalf("download 内容不一致: %q err=%v", data, err)
	}
	var chmod fileop.ChmodResult
	decode("file.chmod", call("file.chmod", `{"path":"up.txt","mode":"0600"}`), &chmod)
	if chmod.Mode != "0600" || chmod.Changed != 1 {
		t.Fatalf("chmod 结果异常: %+v", chmod)
	}
	var mv fileop.MoveResult
	decode("file.move", call("file.move", `{"src":"up.txt","dst":"moved.txt"}`), &mv)
	if !mv.Moved || mv.Method != "rename" {
		t.Fatalf("move 结果异常: %+v", mv)
	}
	var packed fileop.CompressResult
	decode("file.compress", call("file.compress", `{"paths":["moved.txt"],"archive_path":"one.zip"}`), &packed)
	if packed.Entries != 1 || packed.Format != "zip" {
		t.Fatalf("compress 结果异常: %+v", packed)
	}
	var extracted fileop.ExtractResult
	decode("file.extract", call("file.extract", `{"archive_path":"one.zip","dest_dir":"out"}`), &extracted)
	if extracted.Extracted != 1 || extracted.Skipped == nil {
		t.Fatalf("extract 结果异常: %+v", extracted)
	}
	if info, err := os.Stat(filepath.Join(root, "out", "moved.txt")); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("解压后权限=%v err=%v，期望 0600", info.Mode().Perm(), err)
	}
	// 裸名兼容面只保留最初的六个方法，L2 方法必须带 file. 前缀。
	resp := dispatchCall(t, cfg, "upload", `{}`)
	if resp.Error == nil || !strings.Contains(resp.Error.Code, "E_NOTFOUND") {
		t.Fatalf("裸 upload 应回 E_NOTFOUND，实际: %+v", resp.Error)
	}
}
