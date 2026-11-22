package fileop

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestExtractRoundTrip 压缩 → 解压 → 逐文件内容与符号链接目标比对。
func TestExtractRoundTrip(t *testing.T) {
	cases := []struct {
		name   string
		params string
	}{
		{"zip", `{"paths":["src"],"archive_path":"out.zip"}`},
		{"tar.gz", `{"paths":["src"],"archive_path":"out.tar.gz","format":"tar.gz"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			buildTree(t, root)
			if perr := mustCall(t, "file.compress", c.params, root, nil); perr != nil {
				t.Fatalf("压缩失败: %v", perr)
			}
			archive := "out.zip"
			if c.name == "tar.gz" {
				archive = "out.tar.gz"
			}
			var got ExtractResult
			params := `{"archive_path":"` + archive + `","dest_dir":"dst"}`
			if perr := mustCall(t, "file.extract", params, root, &got); perr != nil {
				t.Fatalf("解压失败: %v", perr)
			}
			if got.Extracted == 0 || got.Bytes == 0 || got.Format == "" || got.Skipped == nil {
				t.Fatalf("解压结果异常: %+v", got)
			}
			assertTreeEqual(t, filepath.Join(root, "src"), filepath.Join(root, "dst", "src"))
		})
	}
}

// TestExtractOverwrite 同名文件两种态：false 跳过并记 skipped，true 覆盖。
func TestExtractOverwrite(t *testing.T) {
	root := t.TempDir()
	file, err := os.Create(filepath.Join(root, "payload.zip"))
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(file)
	writer, err := zw.Create("a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("new")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "dst"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "dst", "a.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	var skipped ExtractResult
	if perr := mustCall(t, "file.extract", `{"archive_path":"payload.zip","dest_dir":"dst"}`, root, &skipped); perr != nil {
		t.Fatalf("解压失败: %v", perr)
	}
	if len(skipped.Skipped) != 1 || skipped.Skipped[0] != "a.txt" {
		t.Fatalf("skipped=%v，期望 [a.txt]", skipped.Skipped)
	}
	if content, _ := os.ReadFile(filepath.Join(root, "dst", "a.txt")); string(content) != "old" {
		t.Fatalf("overwrite=false 时内容被改: %q", content)
	}
	var replaced ExtractResult
	if perr := mustCall(t, "file.extract",
		`{"archive_path":"payload.zip","dest_dir":"dst","overwrite":true}`, root, &replaced); perr != nil {
		t.Fatalf("覆盖解压失败: %v", perr)
	}
	if len(replaced.Skipped) != 0 {
		t.Fatalf("overwrite=true 不应有 skipped: %v", replaced.Skipped)
	}
	if content, _ := os.ReadFile(filepath.Join(root, "dst", "a.txt")); string(content) != "new" {
		t.Fatalf("overwrite=true 后内容=%q，期望 new", content)
	}
}

// assertTreeEqual 逐项比对目录树：目录存在、文件字节一致、符号链接目标一致。
func assertTreeEqual(t *testing.T, want, got string) {
	t.Helper()
	wantCount, gotCount := 0, 0
	err := filepath.WalkDir(want, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(want, p)
		if err != nil {
			return err
		}
		wantCount++
		if rel == "." {
			return nil
		}
		target := filepath.Join(got, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case info.IsDir():
			stat, err := os.Stat(target)
			if err != nil || !stat.IsDir() {
				t.Fatalf("目录 %s 未还原: %v", rel, err)
			}
		case info.Mode()&fs.ModeSymlink != 0:
			wantLink, _ := os.Readlink(p)
			gotLink, err := os.Readlink(target)
			if err != nil || gotLink != wantLink {
				t.Fatalf("链接 %s 目标=%q err=%v，期望 %q", rel, gotLink, err, wantLink)
			}
		default:
			wantData, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			gotData, err := os.ReadFile(target)
			if err != nil || !bytes.Equal(wantData, gotData) {
				t.Fatalf("文件 %s 内容不一致: err=%v", rel, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("遍历源树失败: %v", err)
	}
	err = filepath.WalkDir(got, func(string, fs.DirEntry, error) error {
		gotCount++
		return nil
	})
	if err != nil || gotCount != wantCount {
		t.Fatalf("解压后条目数=%d，源树=%d（err=%v）", gotCount, wantCount, err)
	}
}
