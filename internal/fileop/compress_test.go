package fileop

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// buildTree 造一棵含中文名、子目录、空目录与符号链接的源树，返回文件内容表。
func buildTree(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{
		"src/hello.txt":      "你好，kore\n",
		"src/中文 名称.txt":      "中文内容",
		"src/sub/nested.bin": "\x00\x01\x02binary",
	}
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "src", "sub", "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("hello.txt", filepath.Join(root, "src", "link")); err != nil {
		t.Fatal(err)
	}
	return files
}

// TestCompressZip 目录树打包：保留相对路径、目录条目、中文名，符号链接记 link 不跟随。
func TestCompressZip(t *testing.T) {
	root := t.TempDir()
	files := buildTree(t, root)
	var got CompressResult
	if perr := mustCall(t, "file.compress", `{"paths":["src"],"archive_path":"out.zip"}`, root, &got); perr != nil {
		t.Fatalf("file.compress 失败: %v", perr)
	}
	if got.Format != formatZip || got.ArchivePath != "out.zip" || got.Entries < 7 {
		t.Fatalf("压缩结果异常: %+v", got)
	}
	info, err := os.Stat(filepath.Join(root, "out.zip"))
	if err != nil || info.Size() != got.SizeBytes {
		t.Fatalf("归档大小不一致: info=%v err=%v result=%d", info, err, got.SizeBytes)
	}
	zr, err := zip.OpenReader(filepath.Join(root, "out.zip"))
	if err != nil {
		t.Fatalf("打开 zip 失败: %v", err)
	}
	defer func() { _ = zr.Close() }()
	seen := map[string]*zip.File{}
	for _, f := range zr.File {
		seen[f.Name] = f
	}
	for _, dir := range []string{"src/", "src/sub/", "src/sub/empty/"} {
		if f := seen[dir]; f == nil || !f.FileInfo().IsDir() {
			t.Fatalf("缺少目录条目 %q: seen=%v", dir, keys(seen))
		}
	}
	for rel, want := range files {
		name := filepath.ToSlash(rel)
		f := seen[name]
		if f == nil {
			t.Fatalf("缺少文件条目 %q", name)
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil || string(data) != want {
			t.Fatalf("条目 %q 内容=%q err=%v，期望 %q", name, data, err, want)
		}
	}
	link := seen["src/link"]
	if link == nil || link.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("符号链接未按 link 记录: %+v", link)
	}
	rc, err := link.Open()
	if err != nil {
		t.Fatal(err)
	}
	target, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil || string(target) != "hello.txt" {
		t.Fatalf("符号链接目标=%q err=%v，期望 hello.txt（不跟随）", target, err)
	}
}

// TestCompressTarGz tar.gz 同样保留目录、中文名与符号链接类型。
func TestCompressTarGz(t *testing.T) {
	root := t.TempDir()
	buildTree(t, root)
	var got CompressResult
	if perr := mustCall(t, "file.compress",
		`{"paths":["src"],"archive_path":"out.tar.gz","format":"tar.gz"}`, root, &got); perr != nil {
		t.Fatalf("tar.gz 压缩失败: %v", perr)
	}
	if got.Format != formatTarGz || got.Entries < 7 {
		t.Fatalf("tar.gz 结果异常: %+v", got)
	}
	file, err := os.Open(filepath.Join(root, "out.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = gz.Close() }()
	reader := tar.NewReader(gz)
	types := map[string]byte{}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("读 tar 失败: %v", err)
		}
		types[header.Name] = header.Typeflag
	}
	if types["src/"] != tar.TypeDir || types["src/sub/empty/"] != tar.TypeDir {
		t.Fatalf("目录条目类型异常: %v", types)
	}
	if types["src/link"] != tar.TypeSymlink {
		t.Fatalf("符号链接条目类型=%v，期望 TypeSymlink", types["src/link"])
	}
	if _, ok := types["src/中文 名称.txt"]; !ok {
		t.Fatalf("中文名条目缺失: %v", types)
	}
}

// TestCompressRejectsArchiveInsideSource 归档不能写进被压缩目录内部。
func TestCompressRejectsArchiveInsideSource(t *testing.T) {
	root := t.TempDir()
	buildTree(t, root)
	params := `{"paths":["src"],"archive_path":"src/out.zip"}`
	_, perr := Call("file.compress", json.RawMessage(params), root, nil)
	if perr == nil || perr.Code != pluginapi.E_INVALID || !strings.Contains(perr.Message, "内部") {
		t.Fatalf("归档写入源目录错误=%v，期望 E_INVALID", perr)
	}
}

// keys 返回 map 的键集合，仅用于失败信息。
func keys(m map[string]*zip.File) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
