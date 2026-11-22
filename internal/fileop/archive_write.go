package fileop

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

// forEachEntry 按输入顺序遍历每个源（含目录自身），条目名保留根相对路径。
// WalkDir 不跟随符号链接，链接按 Linkname 记一条，不复制目标内容。
func forEachEntry(sources []CompressSource, fn func(name string, info fs.FileInfo, link, srcPath string) error) error {
	for _, src := range sources {
		err := filepath.WalkDir(src.Abs, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(src.Abs, p)
			if err != nil {
				return err
			}
			name := src.Rel
			if rel != "." {
				name = path.Join(src.Rel, filepath.ToSlash(rel))
			}
			link := ""
			switch {
			case info.Mode()&fs.ModeSymlink != 0:
				if link, err = os.Readlink(p); err != nil {
					return err
				}
			case info.IsDir(), info.Mode().IsRegular():
			default:
				return newOpError(ErrUnsupported, "不支持压缩设备/FIFO 等特殊文件")
			}
			return fn(name, info, link, p)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// writeZip 写 zip：目录条目以 "/" 结尾，符号链接用 ModeSymlink + 目标内容表示。
func writeZip(dst io.Writer, sources []CompressSource) (int, error) {
	zw := zip.NewWriter(dst)
	count := 0
	err := forEachEntry(sources, func(name string, info fs.FileInfo, link, srcPath string) error {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: info.ModTime()}
		switch {
		case info.IsDir():
			header.Name = name + "/"
			header.SetMode(info.Mode() | fs.ModeDir)
		case link != "":
			header.SetMode(fs.ModeSymlink | 0o777)
		default:
			header.SetMode(info.Mode().Perm())
		}
		writer, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		count++
		switch {
		case link != "":
			_, err = io.WriteString(writer, link)
			return err
		case info.Mode().IsRegular():
			return copyFileInto(writer, srcPath)
		}
		return nil
	})
	if err != nil {
		_ = zw.Close()
		return 0, err
	}
	if err := zw.Close(); err != nil {
		return 0, err
	}
	return count, nil
}

// writeTar 写 tar 或 tar.gz：PAX 格式承载中文与长路径，属主统一归零。
func writeTar(dst io.Writer, sources []CompressSource, gzipped bool) (int, error) {
	var buf io.Writer = dst
	var gz *gzip.Writer
	if gzipped {
		gz = gzip.NewWriter(dst)
		buf = gz
	}
	tw := tar.NewWriter(buf)
	count := 0
	err := forEachEntry(sources, func(name string, info fs.FileInfo, link, srcPath string) error {
		header := &tar.Header{
			Name:    name,
			Mode:    int64(info.Mode().Perm()),
			ModTime: info.ModTime(),
			Format:  tar.FormatPAX,
		}
		switch {
		case info.IsDir():
			header.Typeflag = tar.TypeDir
			header.Name = name + "/"
		case link != "":
			header.Typeflag = tar.TypeSymlink
			header.Linkname = link
		default:
			header.Typeflag = tar.TypeReg
			header.Size = info.Size()
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		count++
		if header.Typeflag == tar.TypeReg {
			return copyFileInto(tw, srcPath)
		}
		return nil
	})
	if err != nil {
		_ = tw.Close()
		if gz != nil {
			_ = gz.Close()
		}
		return 0, err
	}
	if err := tw.Close(); err != nil {
		if gz != nil {
			_ = gz.Close()
		}
		return 0, err
	}
	if gz != nil {
		if err := gz.Close(); err != nil {
			return 0, err
		}
	}
	return count, nil
}

// copyFileInto 流式拷贝文件内容，不把大文件整体读进内存。
func copyFileInto(dst io.Writer, src string) error {
	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	_, err = io.Copy(dst, file)
	return err
}
