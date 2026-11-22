package e2e

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pengtang1024/kore-plugin-file/internal/fileop"
	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// TestFilePluginL2E2E 真实二进制 + UDS：压缩/解压往返、diff、上传/下载、chmod、move、恶意包拒绝。
func TestFilePluginL2E2E(t *testing.T) {
	bin := buildPlugin(t)
	root := t.TempDir()
	src := filepath.Join(root, "src")
	writeTree(t, src)
	session := startPlugin(t, bin, root)

	var packed fileop.CompressResult
	decode(t, session.must("file.compress", `{"paths":["src"],"archive_path":"out.zip"}`), &packed)
	if packed.Format != "zip" || packed.Entries < 6 || packed.SizeBytes <= 0 {
		t.Fatalf("zip 压缩结果异常: %+v", packed)
	}
	var zi fileop.ExtractResult
	decode(t, session.must("file.extract", `{"archive_path":"out.zip","dest_dir":"dst"}`), &zi)
	if zi.Extracted < 6 || zi.Format != "zip" {
		t.Fatalf("zip 解压结果异常: %+v", zi)
	}
	assertSameTree(t, src, filepath.Join(root, "dst", "src"))

	var tg fileop.CompressResult
	decode(t, session.must("file.compress",
		`{"paths":["src"],"archive_path":"out.tar.gz","format":"tar.gz"}`), &tg)
	if tg.Format != "tar.gz" {
		t.Fatalf("tar.gz 压缩结果异常: %+v", tg)
	}
	var ti fileop.ExtractResult
	decode(t, session.must("file.extract", `{"archive_path":"out.tar.gz","dest_dir":"dst2"}`), &ti)
	assertSameTree(t, src, filepath.Join(root, "dst2", "src"))

	payload, err := os.ReadFile(filepath.Join(src, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	upload := fmt.Sprintf(`{"path":"dst","filename":"hi.txt","data_base64":%q}`,
		base64.StdEncoding.EncodeToString(payload))
	var up fileop.UploadResult
	decode(t, session.must("file.upload", upload), &up)
	sum := sha256.Sum256(payload)
	if up.SHA256 != hex.EncodeToString(sum[:]) || up.Size != int64(len(payload)) {
		t.Fatalf("upload 结果异常: %+v", up)
	}
	var down fileop.DownloadResult
	decode(t, session.must("file.download", `{"path":"dst/hi.txt"}`), &down)
	decoded, err := base64.StdEncoding.DecodeString(down.DataBase64)
	if err != nil || !bytes.Equal(decoded, payload) {
		t.Fatalf("download 内容与上传不一致: err=%v", err)
	}
	downSum := sha256.Sum256(decoded)
	t.Logf("SHA256 upload=%s download=%s", up.SHA256, hex.EncodeToString(downSum[:]))
	if hex.EncodeToString(downSum[:]) != up.SHA256 {
		t.Fatal("上传/下载 sha256 不一致")
	}

	var chmod fileop.ChmodResult
	decode(t, session.must("file.chmod", `{"path":"dst/hi.txt","mode":"0600"}`), &chmod)
	var stat fileop.StatResult
	decode(t, session.must("file.stat", `{"path":"dst/hi.txt"}`), &stat)
	if stat.Mode != "0600" {
		t.Fatalf("chmod 后 mode=%q，期望 0600", stat.Mode)
	}

	var moved fileop.MoveResult
	decode(t, session.must("file.move", `{"src":"dst/hi.txt","dst":"dst/moved.txt"}`), &moved)
	if !moved.Moved {
		t.Fatalf("move 结果异常: %+v", moved)
	}
	session.must("file.stat", `{"path":"dst/moved.txt"}`)
	if _, perr := session.call("file.stat", `{"path":"dst/hi.txt"}`); perr == nil || perr.Code != pluginapi.E_NOTFOUND {
		t.Fatalf("旧路径 stat 错误=%v，期望 E_NOTFOUND", perr)
	}

	writeEvilZip(t, filepath.Join(root, "evil.zip"))
	_, perr := session.call("file.extract", `{"archive_path":"evil.zip","dest_dir":"evil-dst"}`)
	if perr == nil || perr.Code != pluginapi.E_INVALID {
		t.Fatalf("恶意 zip 解压错误=%v，期望 E_INVALID", perr)
	}
	t.Logf("STEP file.extract evil.zip code=%s message=%s", perr.Code, perr.Message)
	if _, err := os.Stat(filepath.Join(root, "evil-dst")); !os.IsNotExist(err) {
		t.Fatalf("恶意包被拒后目标目录仍有残留: %v", err)
	}
	t.Logf("STEP residue-check evil-dst 不存在、../evil.txt 不存在：通过")
	if _, err := os.Stat(filepath.Join(root, "evil.txt")); !os.IsNotExist(err) {
		t.Fatalf("恶意包写出了 ../evil.txt: %v", err)
	}
	session.shutdown()
}

// decode 把步骤结果解成显式结果结构。
func decode(t *testing.T, raw json.RawMessage, out any) {
	t.Helper()
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("结果解码失败: %v（raw=%s）", err, raw)
	}
}

// writeTree 造 E2E 源树：子目录、中文名、空目录与 3 MiB 大文件。
func writeTree(t *testing.T, src string) {
	t.Helper()
	dirs := []string{"", "sub", "sub/深层 目录", "empty"}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(src, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"hello.txt":        "hello kore file L2\n",
		"中文 名称.txt":        "中文内容 with spaces",
		"sub/deep.txt":     "deep",
		"sub/深层 目录/文件.bin": "binary-ish content",
	}
	for rel, content := range files {
		if err := os.WriteFile(filepath.Join(src, filepath.FromSlash(rel)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	big := make([]byte, 3<<20)
	for i := range big {
		big[i] = byte((i*31 + 7) % 251)
	}
	if err := os.WriteFile(filepath.Join(src, "big.bin"), big, 0o644); err != nil {
		t.Fatal(err)
	}
}

// assertSameTree 用系统 diff -r 比对两棵树，并把命令与退出码写进测试日志。
func assertSameTree(t *testing.T, want, got string) {
	t.Helper()
	output, err := exec.Command("diff", "-r", want, got).CombinedOutput()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("执行 diff 失败: %v", err)
		}
		code = exitErr.ExitCode()
	}
	t.Logf("STEP diff -r %s %s exit=%d output=%q", want, got, code, strings.TrimSpace(string(output)))
	if code != 0 {
		t.Fatalf("diff -r 退出码=%d（期望 0）", code)
	}
}

// writeEvilZip 造含 ../evil.txt 的恶意 zip。
func writeEvilZip(t *testing.T, path string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(file)
	writer, err := zw.Create("../evil.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("evil")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
