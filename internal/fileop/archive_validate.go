package fileop

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"io/fs"
	"os"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// validateArchive 在落盘前把整包读完并逐条校验：任何一条非法都整包拒绝，目标零残留。
func validateArchive(req ExtractRequest) error {
	switch req.Format {
	case formatZip:
		return validateZipArchive(req)
	case formatTar, formatTarGz:
		return validateTarArchive(req)
	}
	return newOpError(ErrInvalid, "不支持的归档格式")
}

// validateTarArchive 只读头、不落盘；tar.Reader 会自动丢弃条目数据。
func validateTarArchive(req ExtractRequest) error {
	reader, closeFn, err := openTar(req.ArchiveAbs, req.Format)
	if err != nil {
		return err
	}
	defer closeFn()
	count := 0
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return newOpError(ErrInvalid, "归档包损坏，无法解压")
		}
		count++
		if count > maxArchiveEntries {
			return newOpError(ErrInvalid, "归档条目数超过上限，整包拒绝")
		}
		name, perr := validateTarHeader(header)
		if perr != nil {
			return opErrorFromProtocol(perr)
		}
		if _, perr := entryAbsPath(req.Root, req.DestRel, name); perr != nil {
			return opErrorFromProtocol(perr)
		}
	}
}

// validateZipArchive 读中央目录逐条校验；zip 必须先读符号链接内容才能校验目标。
func validateZipArchive(req ExtractRequest) error {
	zr, err := zip.OpenReader(req.ArchiveAbs)
	if err != nil {
		return newOpError(ErrInvalid, "zip 归档损坏或不是合法 zip")
	}
	defer func() { _ = zr.Close() }()
	if len(zr.File) > maxArchiveEntries {
		return newOpError(ErrInvalid, "归档条目数超过上限，整包拒绝")
	}
	for _, file := range zr.File {
		name, perr := validateZipEntry(file)
		if perr != nil {
			return opErrorFromProtocol(perr)
		}
		if _, perr := entryAbsPath(req.Root, req.DestRel, name); perr != nil {
			return opErrorFromProtocol(perr)
		}
	}
	return nil
}

// validateTarHeader 对照 backup 的 validateRestoreHeader：名字六层 + 类型白名单 +
// 单条目大小 + 符号链接目标归属，任何一条不满足都整包拒绝。
func validateTarHeader(header *tar.Header) (string, *pluginapi.Error) {
	name, perr := validateEntryName(header.Name)
	if perr != nil {
		return "", perr
	}
	switch header.Typeflag {
	case tar.TypeDir, tar.TypeReg, tar.TypeRegA:
	case tar.TypeSymlink:
		if perr := validateLinkTarget(name, header.Linkname); perr != nil {
			return "", perr
		}
	case tar.TypeChar, tar.TypeBlock:
		return "", invalid("归档包含字符/块设备条目，整包拒绝")
	default:
		return "", invalid("归档包含不支持的条目类型，整包拒绝")
	}
	if header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA {
		if header.Size < 0 || header.Size > maxArchiveEntryBytes {
			return "", invalid("归档条目大小超过 2 GiB 上限，整包拒绝")
		}
	}
	return name, nil
}

// validateZipEntry 与 tar 同名六层 + 类型白名单 + 大小上限；符号链接目标单独校验。
func validateZipEntry(file *zip.File) (string, *pluginapi.Error) {
	name, perr := validateEntryName(file.Name)
	if perr != nil {
		return "", perr
	}
	mode := file.Mode()
	switch {
	case file.FileInfo().IsDir():
		if file.UncompressedSize64 != 0 {
			return "", invalid("zip 目录条目带内容，整包拒绝")
		}
	case mode&fs.ModeSymlink != 0:
		target, perr := readZipLink(file)
		if perr != nil {
			return "", perr
		}
		if perr := validateLinkTarget(name, target); perr != nil {
			return "", perr
		}
	case mode.IsRegular():
		if file.UncompressedSize64 > uint64(maxArchiveEntryBytes) {
			return "", invalid("归档条目大小超过 2 GiB 上限，整包拒绝")
		}
	default:
		return "", invalid("zip 包含设备/FIFO 等特殊条目，整包拒绝")
	}
	return name, nil
}

// readZipLink 读取符号链接条目的目标，长度有上限，避免把大文件当链接读进内存。
func readZipLink(file *zip.File) (string, *pluginapi.Error) {
	if file.UncompressedSize64 > uint64(maxLinkTargetLen) {
		return "", invalid("符号链接目标超长，整包拒绝")
	}
	reader, err := file.Open()
	if err != nil {
		return "", invalid("zip 归档损坏，无法读取符号链接")
	}
	defer func() { _ = reader.Close() }()
	buf, err := io.ReadAll(io.LimitReader(reader, maxLinkTargetLen+1))
	if err != nil {
		return "", invalid("zip 归档损坏，无法读取符号链接")
	}
	if len(buf) > maxLinkTargetLen {
		return "", invalid("符号链接目标超长，整包拒绝")
	}
	return string(buf), nil
}

// openTar 按格式打开归档；tar.gz 先过 gzip，坏头按 E_INVALID 处理。
func openTar(abs, format string) (*tar.Reader, func(), error) {
	file, err := os.Open(abs)
	if err != nil {
		return nil, nil, err
	}
	var reader io.Reader = file
	var gz *gzip.Reader
	if format == formatTarGz {
		gz, err = gzip.NewReader(file)
		if err != nil {
			_ = file.Close()
			return nil, nil, newOpError(ErrInvalid, "gzip 归档损坏，无法解压")
		}
		reader = gz
	}
	closeFn := func() {
		if gz != nil {
			_ = gz.Close()
		}
		_ = file.Close()
	}
	return tar.NewReader(reader), closeFn, nil
}
