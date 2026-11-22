package fileop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// compressParams 是 file.compress 的显式入参形状。
type compressParams struct {
	Paths       []string `json:"paths"`
	ArchivePath string   `json:"archive_path"`
	Format      string   `json:"format"`
}

// CompressSource 是一个待压缩条目：Abs 为已校验绝对路径，Rel 为回显与归档内相对路径。
type CompressSource struct {
	Abs string
	Rel string
}

// CompressResult 是 file.compress 的结果。
type CompressResult struct {
	ArchivePath string `json:"archive_path"`
	SizeBytes   int64  `json:"size_bytes"`
	Entries     int    `json:"entries"`
	Format      string `json:"format"`
}

// CompressData 是 file.compress 执行面返回的写后元信息。
type CompressData struct {
	Size    int64
	Entries int
}

// callCompress 处理 file.compress：多路径递归打包、保留相对路径、不跟随符号链接。
func callCompress(raw json.RawMessage, root string, exec Executor) (json.RawMessage, *pluginapi.Error) {
	var p compressParams
	if perr := decodeParams(raw, &p); perr != nil {
		return nil, perr
	}
	if p.ArchivePath == "" {
		return nil, invalid("archive_path 不能为空")
	}
	format, perr := parseCompressFormat(p.Format)
	if perr != nil {
		return nil, perr
	}
	if len(p.Paths) == 0 {
		return nil, invalid("paths 不能为空")
	}
	archiveDisplay := displayPath(p.ArchivePath)
	if archiveDisplay == "." {
		return nil, invalid("archive_path 不能是配置根目录")
	}
	archiveAbs, perr := ResolveTarget(root, p.ArchivePath)
	if perr != nil {
		return nil, perr
	}
	sources, perr := resolveSources(root, p.Paths)
	if perr != nil {
		return nil, perr
	}
	if perr := checkArchivePlacement(archiveDisplay, sources); perr != nil {
		return nil, perr
	}
	data, err := exec.Compress(sources, archiveAbs, format)
	if err != nil {
		return nil, classify(err)
	}
	return encode(CompressResult{
		ArchivePath: archiveDisplay,
		SizeBytes:   data.Size,
		Entries:     data.Entries,
		Format:      format,
	})
}

// resolveSources 逐个校验输入路径并转成 (绝对路径, 回显相对路径) 对，重复项去重。
func resolveSources(root string, raws []string) ([]CompressSource, *pluginapi.Error) {
	sources := make([]CompressSource, 0, len(raws))
	seen := make(map[string]bool, len(raws))
	for _, rawPath := range raws {
		if rawPath == "" {
			return nil, invalid("paths 不能包含空路径")
		}
		display := displayPath(rawPath)
		if display == "." {
			return nil, invalid("paths 不能包含配置根目录")
		}
		if seen[display] {
			continue
		}
		seen[display] = true
		abs, perr := ResolveEntry(root, rawPath)
		if perr != nil {
			return nil, perr
		}
		sources = append(sources, CompressSource{Abs: abs, Rel: display})
	}
	return sources, nil
}

// checkArchivePlacement 阻止把归档写进被压缩目录内部（否则会边压缩边把自身读进去）。
func checkArchivePlacement(archiveDisplay string, sources []CompressSource) *pluginapi.Error {
	for _, src := range sources {
		if archiveDisplay == src.Rel || strings.HasPrefix(archiveDisplay, src.Rel+"/") {
			return invalid("archive_path 不能位于被压缩路径内部")
		}
	}
	return nil
}

// Compress 递归打包到同目录临时文件，fsync 后 rename，失败不留半个归档。
func (realFS) Compress(sources []CompressSource, archiveAbs, format string) (CompressData, error) {
	if info, err := os.Lstat(archiveAbs); err == nil && info.IsDir() {
		return CompressData{}, newOpError(ErrConflict, "归档目标已存在且是目录")
	}
	tmp, err := os.CreateTemp(filepath.Dir(archiveAbs), ".kore-archive-*.tmp")
	if err != nil {
		return CompressData{}, err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	var entries int
	switch format {
	case formatZip:
		entries, err = writeZip(tmp, sources)
	case formatTar, formatTarGz:
		entries, err = writeTar(tmp, sources, format == formatTarGz)
	default:
		err = newOpError(ErrInvalid, "不支持的归档格式")
	}
	if err != nil {
		_ = tmp.Close()
		cleanup()
		return CompressData{}, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return CompressData{}, err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return CompressData{}, err
	}
	if err := os.Rename(tmpName, archiveAbs); err != nil {
		cleanup()
		return CompressData{}, err
	}
	info, err := os.Stat(archiveAbs)
	if err != nil {
		return CompressData{}, err
	}
	return CompressData{Size: info.Size(), Entries: entries}, nil
}
