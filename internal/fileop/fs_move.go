package fileop

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// renamePath 是覆盖语义（overwrite=true）的落点；测试可替换它模拟跨文件系统失败（EXDEV）。
var renamePath = os.Rename

// renameNoReplacePath 是「目标不存在才改名」的原子落点：overwrite=false 的并发竞争
// 全部在这里收口（Linux 走 renameat2(RENAME_NOREPLACE)，其它平台见 rename_noreplace.go）。
// 独立成变量是为了让测试能分别注入 EXDEV 与并发窗口，不改变生产路径语义。
var renameNoReplacePath = renameNoReplace

// Move 先尝试 rename（同文件系统、单次调用），EXDEV 时回退「复制到临时项 + 落名 + 删除源」。
//
// 冲突语义：overwrite=false 时不能只靠 checkMoveTarget 的 Lstat 预检——预检与落名之间
// 目标可能被其它进程/实例创建，os.Rename 会静默覆盖。因此 overwrite=false 的落名改走
// renameNoReplace（原子、目标存在即 EEXIST），跨设备回退的最终落名同样走它。
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
	if overwrite {
		err = renamePath(absSrc, absDst)
	} else {
		err = renameNoReplacePath(absSrc, absDst)
	}
	if err == nil {
		return MoveData{Method: "rename"}, nil
	}
	err = mapRenameError(err)
	if !isCrossDevice(err) {
		return MoveData{}, err
	}
	if err := copyAndDelete(absSrc, absDst, srcInfo, overwrite); err != nil {
		return MoveData{}, mapRenameError(err)
	}
	return MoveData{Method: "copy+delete"}, nil
}

// mapRenameError 把落名阶段的「目标已存在」统一映射为 E_CONFLICT，
// 与 checkMoveTarget 的预检消息保持一致（预检只是快路径，原子落名才是判据）。
func mapRenameError(err error) error {
	if errors.Is(err, fs.ErrExist) {
		return newOpError(ErrConflict, "目标已存在，overwrite=true 可覆盖")
	}
	return err
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

// copyAndDelete 跨文件系统回退：先复制到目标目录的临时项，成功后落名并删除源。
// overwrite=false 时最终落名必须原子且不覆盖（目标可能在复制期间被别的实例创建）。
func copyAndDelete(absSrc, absDst string, srcInfo fs.FileInfo, overwrite bool) error {
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
	if err := replaceInto(tmpName, absDst, overwrite); err != nil {
		_ = os.RemoveAll(tmpName)
		return err
	}
	return os.RemoveAll(absSrc)
}

// replaceInto 把临时项落到目标：
//   - overwrite=true：先删除已校验过的目标（文件 / 空目录）再改名，保持旧语义；
//   - overwrite=false：走 renameNoReplace 原子落名，目标已存在则返回 fs.ErrExist，
//     由上层映射 E_CONFLICT，绝不覆盖别人刚创建的文件。
func replaceInto(tmpName, absDst string, overwrite bool) error {
	if !overwrite {
		return renameNoReplacePath(tmpName, absDst)
	}
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
