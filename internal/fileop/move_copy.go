package fileop

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// copyTree 递归复制目录树：不跟随符号链接，链接按原目标重建。
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case info.IsDir():
			return os.MkdirAll(target, info.Mode().Perm())
		case info.Mode()&fs.ModeSymlink != 0:
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case info.Mode().IsRegular():
			file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
			if err != nil {
				return err
			}
			copyErr := copyFileContent(p, file)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			return closeErr
		default:
			return newOpError(ErrUnsupported, "不支持移动设备/FIFO 等特殊文件")
		}
	})
}

// copyFileContent 流式拷贝文件内容，不把大文件整体读进内存。
func copyFileContent(src string, dst *os.File) error {
	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	buf := make([]byte, 32<<10)
	for {
		n, readErr := file.Read(buf)
		if n > 0 {
			if _, err := dst.Write(buf[:n]); err != nil {
				return err
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return nil
			}
			return readErr
		}
	}
}
