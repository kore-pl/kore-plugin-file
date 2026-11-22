package fileop

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// renameFile 是原子替换的落点；测试可替换它模拟 rename 失败（验证失败不留半文件）。
var renameFile = os.Rename

// Write 原子写入文件：同目录临时文件 → chmod → 写入 → fsync → rename。
// 父目录缺失按 createIfMissing 决定 MkdirAll 或返回 E_NOTFOUND。
func (realFS) Write(abs string, content string, createIfMissing bool) (WriteData, error) {
	info, err := os.Lstat(abs)
	existed := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return WriteData{}, err
	}
	if existed && info.IsDir() {
		return WriteData{}, newOpError(ErrConflict, "目标是目录，不能写入")
	}
	if err := ensureParentDir(filepath.Dir(abs), createIfMissing); err != nil {
		return WriteData{}, err
	}
	perm := DefaultFileMode.Perm()
	if existed {
		perm = info.Mode().Perm()
	}
	if err := atomicWrite(abs, content, perm); err != nil {
		return WriteData{}, err
	}
	written, err := os.Stat(abs)
	if err != nil {
		return WriteData{}, err
	}
	return WriteData{Size: int64(len(content)), ModTime: written.ModTime(), Created: !existed}, nil
}

// ensureParentDir 校验 / 创建父目录。
func ensureParentDir(dir string, createIfMissing bool) error {
	info, err := os.Stat(dir)
	if err == nil {
		if !info.IsDir() {
			return newOpError(ErrConflict, "父路径不是目录")
		}
		return nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if !createIfMissing {
		return newOpError(ErrNotFound, "父目录不存在，设置 create_if_missing=true 可自动创建")
	}
	return os.MkdirAll(dir, DefaultDirMode)
}

// atomicWrite 实现原子替换：失败时删除临时文件，目标文件保持原内容。
func atomicWrite(path string, content string, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".kore-file-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
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
	if err := renameFile(tmpName, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

// Delete 删文件或目录：非空目录在 recursive=false 时回 E_CONFLICT，
// recursive=true 走 RemoveAll（不跟随符号链接）。
func (realFS) Delete(abs string, recursive bool) error {
	info, err := os.Lstat(abs)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return os.Remove(abs)
	}
	if !recursive {
		items, err := os.ReadDir(abs)
		if err != nil {
			return err
		}
		if len(items) > 0 {
			return newOpError(ErrConflict, "目录非空，recursive=true 可递归删除")
		}
		return os.Remove(abs)
	}
	return os.RemoveAll(abs)
}

// Mkdir 创建单层目录：目标已存在回 E_CONFLICT，父目录不存在回 E_NOTFOUND。
func (realFS) Mkdir(abs string, mode fs.FileMode) error {
	if _, err := os.Lstat(abs); err == nil {
		return newOpError(ErrConflict, "目标已存在")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(abs)
	info, err := os.Stat(parent)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return newOpError(ErrNotFound, "父目录不存在")
		}
		return err
	}
	if !info.IsDir() {
		return newOpError(ErrConflict, "父路径不是目录")
	}
	return os.Mkdir(abs, mode)
}
