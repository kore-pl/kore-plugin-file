package fileop

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// renamePath 是移动的落点；测试可替换它模拟跨文件系统失败（EXDEV）。
var renamePath = os.Rename

// Move 先尝试 rename（原子、同文件系统），EXDEV 时回退「复制到临时项 + rename + 删除源」。
func (realFS) Move(absSrc, absDst string, overwrite bool) (MoveData, error) {
	srcInfo, err := os.Lstat(absSrc)
	if err != nil {
		return MoveData{}, err
	}
	parent, err := os.Stat(filepath.Dir(absDst))
	if err != nil {
		return MoveData{}, err
	}
	if !parent.IsDir() {
		return MoveData{}, newOpError(ErrInvalid, "目标父路径不是目录")
	}
	if err := checkMoveTarget(absDst, srcInfo, overwrite); err != nil {
		return MoveData{}, err
	}
	err = renamePath(absSrc, absDst)
	if err == nil {
		return MoveData{Method: "rename"}, nil
	}
	if !isCrossDevice(err) {
		return MoveData{}, err
	}
	if err := copyAndDelete(absSrc, absDst, srcInfo); err != nil {
		return MoveData{}, err
	}
	return MoveData{Method: "copy+delete"}, nil
}

// checkMoveTarget 校验目标已存在时的覆盖策略；目录只允许覆盖空目录。
func checkMoveTarget(absDst string, srcInfo fs.FileInfo, overwrite bool) error {
	dstInfo, err := os.Lstat(absDst)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !overwrite {
		return newOpError(ErrConflict, "目标已存在，overwrite=true 可覆盖")
	}
	if srcInfo.IsDir() != dstInfo.IsDir() {
		return newOpError(ErrConflict, "目标已存在且类型不同（文件/目录），不能覆盖")
	}
	if dstInfo.IsDir() {
		entries, err := os.ReadDir(absDst)
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			return newOpError(ErrConflict, "目标目录非空，不能覆盖")
		}
	}
	return nil
}

// copyAndDelete 跨文件系统回退：先复制到目标目录的临时项，成功后替换目标并删除源。
func copyAndDelete(absSrc, absDst string, srcInfo fs.FileInfo) error {
	dir := filepath.Dir(absDst)
	var tmpName string
	var err error
	switch {
	case srcInfo.Mode()&fs.ModeSymlink != 0:
		tmpName, err = copySymlinkToTemp(absSrc, dir)
	case srcInfo.IsDir():
		tmpName, err = copyDirToTemp(absSrc, dir)
	default:
		tmpName, err = copyFileToTemp(absSrc, dir, srcInfo.Mode().Perm())
	}
	if err != nil {
		return err
	}
	if err := replaceInto(tmpName, absDst); err != nil {
		_ = os.RemoveAll(tmpName)
		return err
	}
	return os.RemoveAll(absSrc)
}

// replaceInto 把临时项落到目标：目标已存在时先删除（前面已校验类型与空目录）。
func replaceInto(tmpName, absDst string) error {
	if err := os.Remove(absDst); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return os.Rename(tmpName, absDst)
}

// copySymlinkToTemp 在目标目录建一个临时链接（链接目标原样保留，不跟随）。
func copySymlinkToTemp(absSrc, dir string) (string, error) {
	target, err := os.Readlink(absSrc)
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".kore-move-*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return "", err
	}
	if err := os.Remove(tmpName); err != nil {
		return "", err
	}
	if err := os.Symlink(target, tmpName); err != nil {
		return "", err
	}
	return tmpName, nil
}

// copyDirToTemp 在目标目录建临时目录并递归复制源树。
func copyDirToTemp(absSrc, dir string) (string, error) {
	tmpName, err := os.MkdirTemp(dir, ".kore-move-*")
	if err != nil {
		return "", err
	}
	if err := copyTree(absSrc, tmpName); err != nil {
		_ = os.RemoveAll(tmpName)
		return "", err
	}
	return tmpName, nil
}

// copyFileToTemp 在目标目录建临时文件并复制内容与权限位。
func copyFileToTemp(absSrc, dir string, mode fs.FileMode) (string, error) {
	tmp, err := os.CreateTemp(dir, ".kore-move-*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return "", err
	}
	if err := copyFileContent(absSrc, tmp); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return "", err
	}
	return tmpName, nil
}
