package fileop

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestRenameNoReplaceKeepsTargetOnConflict 原子落名的核心契约：
// 目标已存在时返回 fs.ErrExist，且**双方文件内容都不变**（不覆盖、不删源）。
func TestRenameNoReplaceKeepsTargetOnConflict(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src.txt")
	dst := filepath.Join(root, "dst.txt")
	if err := os.WriteFile(src, []byte("src"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("dst"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := renameNoReplace(src, dst)
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("目标已存在时错误=%v，期望 fs.ErrExist", err)
	}
	for path, want := range map[string]string{src: "src", dst: "dst"} {
		got, readErr := os.ReadFile(path)
		if readErr != nil || string(got) != want {
			t.Fatalf("%s 内容=%q err=%v，期望 %q（冲突时不允许落名）", path, got, readErr, want)
		}
	}
}

// TestReplaceIntoRespectsOverwrite 复用落名函数的两个调用方语义：
// overwrite=false 时目标已存在返回 fs.ErrExist 且目标内容不变；overwrite=true 时替换。
func TestReplaceIntoRespectsOverwrite(t *testing.T) {
	root := t.TempDir()
	tmp := filepath.Join(root, ".kore-move-x.tmp")
	dst := filepath.Join(root, "dst.txt")
	if err := os.WriteFile(tmp, []byte("tmp"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("dst"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := replaceInto(tmp, dst, false); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("overwrite=false 错误=%v，期望 fs.ErrExist", err)
	}
	if got, err := os.ReadFile(dst); err != nil || string(got) != "dst" {
		t.Fatalf("overwrite=false 覆盖了目标：内容=%q err=%v", got, err)
	}
	if err := replaceInto(tmp, dst, true); err != nil {
		t.Fatalf("overwrite=true 落名失败: %v", err)
	}
	if got, err := os.ReadFile(dst); err != nil || string(got) != "tmp" {
		t.Fatalf("overwrite=true 后目标内容=%q err=%v，期望 tmp", got, err)
	}
}
