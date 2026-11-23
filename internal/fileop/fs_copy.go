package fileop

import (
	"io/fs"
	"os"
	"path/filepath"
)

// Copy 复制文件 / 目录 / 符号链接：先落到目标目录的临时项，再 rename 就位，
// 失败不留半个副本；目录递归复制且不跟随符号链接。
func (realFS) Copy(absSrc, absDst string, overwrite bool) (CopyData, error) {
	srcInfo, err := os.Lstat(absSrc)
	if err != nil {
		return CopyData{}, err
	}
	parent, err := os.Stat(filepath.Dir(absDst))
	if err != nil {
		return CopyData{}, err
	}
	if !parent.IsDir() {
		return CopyData{}, newOpError(ErrInvalid, "目标父路径不是目录")
	}
	if err := checkMoveTarget(absDst, srcInfo, overwrite); err != nil {
		return CopyData{}, err
	}
	dir := filepath.Dir(absDst)
	var tmpName string
	switch {
	case srcInfo.Mode()&fs.ModeSymlink != 0:
		tmpName, err = copySymlinkToTemp(absSrc, dir)
	case srcInfo.IsDir():
		tmpName, err = copyDirToTemp(absSrc, dir)
	case srcInfo.Mode().IsRegular():
		tmpName, err = copyFileToTemp(absSrc, dir, srcInfo.Mode().Perm())
	default:
		return CopyData{}, newOpError(ErrUnsupported, "不支持复制设备/FIFO 等特殊文件")
	}
	if err != nil {
		return CopyData{}, err
	}
	if err := replaceInto(tmpName, absDst); err != nil {
		_ = os.RemoveAll(tmpName)
		return CopyData{}, err
	}
	entries, bytes, err := measureCopy(absSrc, srcInfo)
	if err != nil {
		return CopyData{}, err
	}
	return CopyData{Kind: copyKind(srcInfo.Mode()), Entries: entries, Bytes: bytes}, nil
}

// measureCopy 统计复制出的条目数与字节数；目录按 WalkDir 重新量一次（不跟随链接）。
func measureCopy(absSrc string, srcInfo fs.FileInfo) (int, int64, error) {
	if !srcInfo.IsDir() {
		return 1, srcInfo.Size(), nil
	}
	entries, total := 0, int64(0)
	err := filepath.WalkDir(absSrc, func(_ string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		entries++
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	return entries, total, nil
}

// copyKind 把文件类型归一为对外 kind 值。
func copyKind(mode fs.FileMode) string {
	switch {
	case mode&fs.ModeSymlink != 0:
		return "symlink"
	case mode.IsDir():
		return "dir"
	default:
		return "file"
	}
}
