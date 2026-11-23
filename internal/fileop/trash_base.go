package fileop

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// trashBase 计算并确保回收站根目录可用（create=true 时按需创建）。
// 约束：必须在配置根的真实路径之内，且自身必须是真实目录（不得是符号链接）——
// 否则删除会把文件搬到根外，等于开了一个新的越界口。
func trashBase(root string, create bool) (string, bool, *pluginapi.Error) {
	realRoot, perr := evalRoot(root)
	if perr != nil {
		return "", false, perr
	}
	base := filepath.Join(realRoot, TrashDirName)
	info, err := os.Lstat(base)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if !create {
			return "", false, nil
		}
		if err := os.MkdirAll(filepath.Join(base, trashEntriesDirName), 0o700); err != nil {
			return "", false, classify(err)
		}
	case err != nil:
		return "", false, classify(err)
	case info.Mode()&fs.ModeSymlink != 0:
		return "", false, invalid("回收站目录 " + TrashDirName + " 是符号链接，拒绝使用")
	case !info.IsDir():
		return "", false, invalid("回收站目录 " + TrashDirName + " 被同名文件占用，请先移走")
	}
	if err := os.Chmod(base, 0o700); err != nil {
		return "", false, classify(err)
	}
	real, err := filepath.EvalSymlinks(base)
	if err != nil {
		return "", false, classify(err)
	}
	if real != base || !within(realRoot, real) {
		return "", false, invalid("回收站路径经符号链接解析后越出配置根目录")
	}
	entries := filepath.Join(base, trashEntriesDirName)
	if create {
		if err := ensureTrashSubdir(entries); err != nil {
			return "", false, classify(err)
		}
	} else if _, err := os.Lstat(entries); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return base, true, nil
		}
		return "", false, classify(err)
	}
	return base, true, nil
}

// ensureTrashSubdir 确保条目目录存在且是真实目录（不跟随符号链接）。
func ensureTrashSubdir(entries string) error {
	info, err := os.Lstat(entries)
	if err == nil {
		if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
			return newOpError(ErrInvalid, "回收站条目目录不可用：必须是配置根内的真实目录")
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	return os.Mkdir(entries, 0o700)
}
