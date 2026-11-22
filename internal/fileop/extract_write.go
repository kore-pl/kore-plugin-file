package fileop

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// applyDir 创建目录；已存在同类型目录视为合并，不吞掉已有内容，也不计 extracted。
func applyDir(abs string, mode fs.FileMode, data *ExtractData) error {
	info, err := os.Lstat(abs)
	switch {
	case err == nil:
		if !info.IsDir() {
			return newOpError(ErrConflict, "归档目录与已有非目录条目同名")
		}
		return nil
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}
	if mode == 0 {
		mode = DefaultDirMode.Perm()
	}
	if err := os.MkdirAll(abs, mode); err != nil {
		return err
	}
	if err := os.Chmod(abs, mode); err != nil {
		return err
	}
	data.Extracted++
	return nil
}

// applyFile 写常规文件：overwrite=false 且目标已存在则记 skipped；
// 内容先写同目录临时文件再 rename，失败不留半个文件。
func applyFile(abs string, src io.Reader, size int64, mode fs.FileMode, overwrite bool, name string, data *ExtractData) error {
	info, err := os.Lstat(abs)
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if exists {
		if info.IsDir() {
			return newOpError(ErrConflict, "归档文件与已有目录同名: "+name)
		}
		if !overwrite {
			data.Skipped = append(data.Skipped, name)
			return nil
		}
	}
	if mode == 0 {
		mode = DefaultFileMode.Perm()
	}
	if err := writeStreamAtomic(abs, src, size, mode); err != nil {
		return err
	}
	data.Bytes += size
	data.Extracted++
	return nil
}

// applySymlink 只重建链接本身，不跟随；overwrite=false 时同名跳过。
func applySymlink(abs, target string, overwrite bool, name string, data *ExtractData) error {
	info, err := os.Lstat(abs)
	if err == nil {
		if info.IsDir() {
			return newOpError(ErrConflict, "归档符号链接与已有目录同名: "+name)
		}
		if !overwrite {
			data.Skipped = append(data.Skipped, name)
			return nil
		}
		if err := os.Remove(abs); err != nil {
			return err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), DefaultDirMode); err != nil {
		return err
	}
	if err := os.Symlink(target, abs); err != nil {
		return err
	}
	data.Extracted++
	return nil
}

// writeStreamAtomic 把流写到同目录临时文件，fsync 后 rename 到目标。
func writeStreamAtomic(abs string, src io.Reader, size int64, mode fs.FileMode) error {
	dir := filepath.Dir(abs)
	if err := os.MkdirAll(dir, DefaultDirMode); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".kore-extract-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if size > 0 {
		if _, err := io.CopyN(tmp, src, size); err != nil {
			_ = tmp.Close()
			cleanup()
			return newOpError(ErrInvalid, "归档条目内容不完整，解压中止")
		}
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, abs); err != nil {
		cleanup()
		return err
	}
	return nil
}
