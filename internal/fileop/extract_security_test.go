package fileop

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// writeMaliciousZip 造一个含穿越条目的 zip（正常库不会替调用方清洗条目名）。
func writeMaliciousZip(t *testing.T, path, name string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(file)
	writer, err := zw.Create(name)
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

// writeMaliciousTar 造一个含穿越文件的 tar 或含逃逸目标的符号链接 tar。
func writeMaliciousTar(t *testing.T, path string, header *tar.Header) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(file)
	if err := tw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if header.Typeflag == tar.TypeReg {
		if _, err := tw.Write(bytes.Repeat([]byte("x"), int(header.Size))); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

// writeMaliciousTgz 把给定 header 包成 tar.gz。
func writeMaliciousTgz(t *testing.T, path string, header *tar.Header) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if header.Typeflag == tar.TypeReg {
		if _, err := tw.Write(bytes.Repeat([]byte("x"), int(header.Size))); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestExtractMaliciousArchive 穿越 / 绝对路径 / 逃逸符号链接一律整包拒绝且目标零残留。
func TestExtractMaliciousArchive(t *testing.T) {
	root := t.TempDir()
	writeMaliciousZip(t, filepath.Join(root, "evil.zip"), "../evil.txt")
	writeMaliciousZip(t, filepath.Join(root, "abs.zip"), "/etc/evil.txt")
	writeMaliciousZip(t, filepath.Join(root, "backslash.zip"), `..\\evil.txt`)
	writeMaliciousTar(t, filepath.Join(root, "evil.tar"), &tar.Header{
		Name: "../evil.txt", Mode: 0o644, Size: 4, Typeflag: tar.TypeReg,
	})
	writeMaliciousTgz(t, filepath.Join(root, "evil.tar.gz"), &tar.Header{
		Name: ".", Mode: 0o755, Typeflag: tar.TypeDir,
	})
	writeMaliciousTgz(t, filepath.Join(root, "link.tar.gz"), &tar.Header{
		Name: "link", Mode: 0o777, Typeflag: tar.TypeSymlink, Linkname: "../../outside",
	})
	writeMaliciousTgz(t, filepath.Join(root, "abs-link.tar.gz"), &tar.Header{
		Name: "link", Mode: 0o777, Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd",
	})
	cases := []struct{ archive, dest string }{
		{"evil.zip", "dst-zip"},
		{"abs.zip", "dst-abs"},
		{"backslash.zip", "dst-backslash"},
		{"evil.tar", "dst-tar"},
		{"evil.tar.gz", "dst-tgz"},
		{"link.tar.gz", "dst-link"},
		{"abs-link.tar.gz", "dst-abs-link"},
	}
	for _, c := range cases {
		t.Run(c.archive, func(t *testing.T) {
			params := `{"archive_path":"` + c.archive + `","dest_dir":"` + c.dest + `"}`
			_, perr := Call("file.extract", json.RawMessage(params), root, nil)
			if perr == nil || perr.Code != pluginapi.E_INVALID {
				t.Fatalf("%s 解压错误=%v，期望 E_INVALID", c.archive, perr)
			}
			if _, err := os.Stat(filepath.Join(root, c.dest)); !os.IsNotExist(err) {
				t.Fatalf("%s 被拒后目标目录仍有残留: %v", c.archive, err)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(root, "evil.txt")); !os.IsNotExist(err) {
		t.Fatalf("穿越条目写出了根内文件: %v", err)
	}
}
