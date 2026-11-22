package fileop

import (
	"archive/tar"
	"archive/zip"
	"io"
	"io/fs"
)

// extractTar 解压 tar / tar.gz：逐条目再做一次 ResolveTarget，落盘前再防一次逃逸。
func extractTar(req ExtractRequest) (ExtractData, error) {
	reader, closeFn, err := openTar(req.ArchiveAbs, req.Format)
	if err != nil {
		return ExtractData{}, err
	}
	defer closeFn()
	data := ExtractData{Format: req.Format}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ExtractData{}, newOpError(ErrInvalid, "归档包损坏，无法解压")
		}
		name, perr := validateTarHeader(header)
		if perr != nil {
			return ExtractData{}, opErrorFromProtocol(perr)
		}
		abs, perr := entryAbsPath(req.Root, req.DestRel, name)
		if perr != nil {
			return ExtractData{}, opErrorFromProtocol(perr)
		}
		mode := fs.FileMode(header.Mode) & 0o777
		switch header.Typeflag {
		case tar.TypeDir:
			if err := applyDir(abs, mode, &data); err != nil {
				return ExtractData{}, err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := applyFile(abs, reader, header.Size, mode, req.Overwrite, name, &data); err != nil {
				return ExtractData{}, err
			}
		case tar.TypeSymlink:
			if err := applySymlink(abs, header.Linkname, req.Overwrite, name, &data); err != nil {
				return ExtractData{}, err
			}
		}
	}
	return data, nil
}

// extractZip 解压 zip：目录、常规文件、符号链接三类，其余类型在校验阶段已拒绝。
func extractZip(req ExtractRequest) (ExtractData, error) {
	zr, err := zip.OpenReader(req.ArchiveAbs)
	if err != nil {
		return ExtractData{}, newOpError(ErrInvalid, "zip 归档损坏或不是合法 zip")
	}
	defer func() { _ = zr.Close() }()
	data := ExtractData{Format: req.Format}
	for _, file := range zr.File {
		name, perr := validateZipEntry(file)
		if perr != nil {
			return ExtractData{}, opErrorFromProtocol(perr)
		}
		abs, perr := entryAbsPath(req.Root, req.DestRel, name)
		if perr != nil {
			return ExtractData{}, opErrorFromProtocol(perr)
		}
		mode := file.Mode().Perm() & 0o777
		switch {
		case file.FileInfo().IsDir():
			if err := applyDir(abs, mode, &data); err != nil {
				return ExtractData{}, err
			}
		case file.Mode()&fs.ModeSymlink != 0:
			target, perr := readZipLink(file)
			if perr != nil {
				return ExtractData{}, opErrorFromProtocol(perr)
			}
			if err := applySymlink(abs, target, req.Overwrite, name, &data); err != nil {
				return ExtractData{}, err
			}
		default:
			reader, err := file.Open()
			if err != nil {
				return ExtractData{}, newOpError(ErrInvalid, "zip 归档损坏，无法读取条目")
			}
			err = applyFile(abs, reader, int64(file.UncompressedSize64), mode, req.Overwrite, name, &data)
			_ = reader.Close()
			if err != nil {
				return ExtractData{}, err
			}
		}
	}
	return data, nil
}
