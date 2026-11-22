package fileop

import (
	"encoding/json"
	"io/fs"
	"os"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// extractParams 是 file.extract 的显式入参形状。
type extractParams struct {
	ArchivePath string `json:"archive_path"`
	DestDir     string `json:"dest_dir"`
	Overwrite   bool   `json:"overwrite"`
}

// ExtractRequest 是解压执行面的完整请求：root 与 dest 同时给出，
// 逐条目复用 ResolveTarget，和 list/read/write 走同一条安全校验路径。
type ExtractRequest struct {
	Root       string
	ArchiveAbs string
	DestRel    string
	DestAbs    string
	Format     string
	Overwrite  bool
}

// ExtractResult 是 file.extract 的结果。
type ExtractResult struct {
	Extracted int      `json:"extracted"`
	Skipped   []string `json:"skipped"`
	Bytes     int64    `json:"bytes"`
	Format    string   `json:"format"`
}

// ExtractData 是 file.extract 执行面返回的原始结果。
type ExtractData struct {
	Extracted int
	Skipped   []string
	Bytes     int64
	Format    string
}

// callExtract 处理 file.extract：先按扩展名定格式，再校验归档与目标目录。
func callExtract(raw json.RawMessage, root string, exec Executor) (json.RawMessage, *pluginapi.Error) {
	var p extractParams
	if perr := decodeParams(raw, &p); perr != nil {
		return nil, perr
	}
	if p.ArchivePath == "" {
		return nil, invalid("archive_path 不能为空")
	}
	if p.DestDir == "" {
		return nil, invalid("dest_dir 不能为空")
	}
	archiveAbs, perr := ResolveExisting(root, p.ArchivePath)
	if perr != nil {
		return nil, perr
	}
	format, perr := detectArchiveFormat(p.ArchivePath)
	if perr != nil {
		return nil, perr
	}
	destDisplay := displayPath(p.DestDir)
	destAbs, perr := ResolveTarget(root, p.DestDir)
	if perr != nil {
		return nil, perr
	}
	data, err := exec.Extract(ExtractRequest{
		Root:       root,
		ArchiveAbs: archiveAbs,
		DestRel:    destDisplay,
		DestAbs:    destAbs,
		Format:     format,
		Overwrite:  p.Overwrite,
	})
	if err != nil {
		return nil, classify(err)
	}
	skipped := data.Skipped
	if skipped == nil {
		skipped = []string{}
	}
	return encode(ExtractResult{
		Extracted: data.Extracted,
		Skipped:   skipped,
		Bytes:     data.Bytes,
		Format:    data.Format,
	})
}

// Extract 先整包校验（不落盘），通过后才创建目标目录并解压；
// 恶意包在第一步就被拒绝，所以目标目录不会被创建，更不会留残片。
func (realFS) Extract(req ExtractRequest) (ExtractData, error) {
	if err := validateArchive(req); err != nil {
		return ExtractData{}, err
	}
	if err := ensureExtractDir(req.DestAbs); err != nil {
		return ExtractData{}, err
	}
	switch req.Format {
	case formatZip:
		return extractZip(req)
	case formatTar, formatTarGz:
		return extractTar(req)
	}
	return ExtractData{}, newOpError(ErrInvalid, "不支持的归档格式")
}

// ensureExtractDir 目标已存在必须是目录，不存在则按 0755 创建。
func ensureExtractDir(abs string) error {
	info, err := os.Lstat(abs)
	if err == nil {
		if !info.IsDir() {
			return newOpError(ErrConflict, "解压目标已存在且不是目录")
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	return os.MkdirAll(abs, fs.FileMode(DefaultDirMode))
}
