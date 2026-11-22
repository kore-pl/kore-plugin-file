package fileop

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// uploadParamsFor 拼一个合法的 file.upload 参数串。
func uploadParamsFor(dir, name string, data []byte) string {
	return fmt.Sprintf(`{"path":%q,"filename":%q,"data_base64":%q}`,
		dir, name, base64.StdEncoding.EncodeToString(data))
}

// TestUploadDownloadRoundTrip 上传二进制（含 NUL）后下载回来逐字节一致，并核对 sha256。
func TestUploadDownloadRoundTrip(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	payload := []byte("hello 上传\x00\x01binary")
	var up UploadResult
	if perr := mustCall(t, "file.upload", uploadParamsFor("docs", "a.bin", payload), root, &up); perr != nil {
		t.Fatalf("file.upload 失败: %v", perr)
	}
	sum := sha256.Sum256(payload)
	if up.Path != "docs/a.bin" || up.Filename != "a.bin" || up.Size != int64(len(payload)) ||
		up.SHA256 != hex.EncodeToString(sum[:]) || !up.Created {
		t.Fatalf("上传结果异常: %+v", up)
	}
	onDisk, err := os.ReadFile(filepath.Join(root, "docs", "a.bin"))
	if err != nil || !bytes.Equal(onDisk, payload) {
		t.Fatalf("落盘内容不一致: err=%v bytes=%d", err, len(onDisk))
	}
	var down DownloadResult
	if perr := mustCall(t, "file.download", `{"path":"docs/a.bin"}`, root, &down); perr != nil {
		t.Fatalf("file.download 失败: %v", perr)
	}
	if down.Filename != "a.bin" || down.SizeBytes != int64(len(payload)) {
		t.Fatalf("下载结果异常: %+v", down)
	}
	decoded, err := base64.StdEncoding.DecodeString(down.DataBase64)
	if err != nil || !bytes.Equal(decoded, payload) {
		t.Fatalf("下载内容不一致: err=%v", err)
	}
}

// TestUploadConflictAndOverwrite 同名上传：默认 E_CONFLICT，overwrite=true 覆盖且继承权限位。
func TestUploadConflictAndOverwrite(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "up"), 0o755); err != nil {
		t.Fatal(err)
	}
	first := uploadParamsFor("up", "a.txt", []byte("one"))
	var created UploadResult
	if perr := mustCall(t, "file.upload", first, root, &created); perr != nil || !created.Created {
		t.Fatalf("首次上传失败: %v", perr)
	}
	if err := os.Chmod(filepath.Join(root, "up", "a.txt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, perr := Call("file.upload", json.RawMessage(first), root, nil); perr == nil || perr.Code != pluginapi.E_CONFLICT {
		t.Fatalf("同名上传错误=%v，期望 E_CONFLICT", perr)
	}
	overwrite := fmt.Sprintf(`{"path":"up","filename":"a.txt","data_base64":%q,"overwrite":true}`,
		base64.StdEncoding.EncodeToString([]byte("two")))
	var replaced UploadResult
	if perr := mustCall(t, "file.upload", overwrite, root, &replaced); perr != nil {
		t.Fatalf("overwrite 上传失败: %v", perr)
	}
	if replaced.Created || replaced.Size != 3 {
		t.Fatalf("覆盖上传结果异常: %+v", replaced)
	}
	onDisk, err := os.ReadFile(filepath.Join(root, "up", "a.txt"))
	if err != nil || string(onDisk) != "two" {
		t.Fatalf("覆盖后内容=%q err=%v", onDisk, err)
	}
	info, err := os.Stat(filepath.Join(root, "up", "a.txt"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("覆盖后权限=%v err=%v，期望 0600", info.Mode().Perm(), err)
	}
}

// TestUploadTooLarge 编码长度超过单帧可承载上限时，解码前就拒绝并说清口径与 L3 分片。
func TestUploadTooLarge(t *testing.T) {
	root := t.TempDir()
	encoded := strings.Repeat("A", base64.StdEncoding.EncodedLen(int(MaxInlineTransferBytes))+8)
	params := fmt.Sprintf(`{"path":".","filename":"big.bin","data_base64":%q}`, encoded)
	_, perr := Call("file.upload", json.RawMessage(params), root, nil)
	if perr == nil || perr.Code != pluginapi.E_INVALID {
		t.Fatalf("超限上传错误=%v，期望 E_INVALID", perr)
	}
	for _, want := range []string{"单次上传上限", "L3", fmt.Sprint(MaxTransferBytes)} {
		if !strings.Contains(perr.Message, want) {
			t.Fatalf("超限提示 %q 未含 %q", perr.Message, want)
		}
	}
}

// TestUploadRejectsUnsafeTargets filename 路径语义、非法 base64 与符号链接目录逃逸全部拒绝。
func TestUploadRejectsUnsafeTargets(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "outlink")); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ name, params string }{
		{"filename-slash", `{"path":"docs","filename":"../evil","data_base64":""}`},
		{"filename-backslash", `{"path":"docs","filename":"a\\b","data_base64":""}`},
		{"filename-dotdot", `{"path":"docs","filename":"..","data_base64":""}`},
		{"path-up", `{"path":"..","filename":"x","data_base64":""}`},
		{"symlink-dir", `{"path":"outlink","filename":"x","data_base64":""}`},
		{"bad-base64", `{"path":"docs","filename":"x","data_base64":"!!!"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, perr := Call("file.upload", json.RawMessage(c.params), root, nil)
			if perr == nil || perr.Code != pluginapi.E_INVALID {
				t.Fatalf("%s 错误=%v，期望 E_INVALID", c.name, perr)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(outside, "x")); !os.IsNotExist(err) {
		t.Fatalf("符号链接逃逸写出了文件: %v", err)
	}
}

// TestDownloadBounds 空文件、目录、不存在与超限四种边界。
func TestDownloadBounds(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "empty.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var empty DownloadResult
	if perr := mustCall(t, "file.download", `{"path":"empty.txt"}`, root, &empty); perr != nil {
		t.Fatalf("空文件下载失败: %v", perr)
	}
	if empty.SizeBytes != 0 || empty.DataBase64 != "" || empty.Filename != "empty.txt" {
		t.Fatalf("空文件下载结果异常: %+v", empty)
	}
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, perr := Call("file.download", json.RawMessage(`{"path":"dir"}`), root, nil); perr == nil || perr.Code != pluginapi.E_INVALID {
		t.Fatalf("目录下载错误=%v，期望 E_INVALID", perr)
	}
	if _, perr := Call("file.download", json.RawMessage(`{"path":"missing"}`), root, nil); perr == nil || perr.Code != pluginapi.E_NOTFOUND {
		t.Fatalf("不存在文件错误=%v，期望 E_NOTFOUND", perr)
	}
	big, err := os.Create(filepath.Join(root, "big.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if err := big.Truncate(MaxInlineTransferBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := big.Close(); err != nil {
		t.Fatal(err)
	}
	_, perr := Call("file.download", json.RawMessage(`{"path":"big.bin"}`), root, nil)
	if perr == nil || perr.Code != pluginapi.E_INVALID || !strings.Contains(perr.Message, "文件过大，请用目录打包下载") {
		t.Fatalf("超限下载错误=%v，期望 E_INVALID + 打包下载提示", perr)
	}
}
