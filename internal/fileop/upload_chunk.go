package fileop

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// uploadChunkParams 是 file.uploadChunk 的显式入参形状。
// offset 是从 0 起的字节偏移，data_base64 是这一段内容；total_size 可选，用于提前拦截越界与判断完成。
type uploadChunkParams struct {
	Path       string `json:"path"`
	Offset     int64  `json:"offset"`
	DataBase64 string `json:"data_base64"`
	TotalSize  *int64 `json:"total_size,omitempty"`
}

// UploadChunkResult 是 file.uploadChunk 的结果。
type UploadChunkResult struct {
	Path      string `json:"path"`
	Received  int64  `json:"received"`
	TotalSize *int64 `json:"total_size,omitempty"`
	Complete  bool   `json:"complete"`
	Replayed  bool   `json:"replayed"`
}

// uploadStatusParams 是 file.uploadStatus 的显式入参形状。
type uploadStatusParams struct {
	Path string `json:"path"`
}

// UploadStatusResult 是 file.uploadStatus 的结果：received 即断点续传应使用的 offset。
type UploadStatusResult struct {
	Path     string `json:"path"`
	Received int64  `json:"received"`
	Exists   bool   `json:"exists"`
}

// callUploadChunk 处理 file.uploadChunk：按 offset 追加/覆盖写入，支持断点续传。
//
// offset 语义（契约，测试锁死）：offset 必须落在 [0, 已收字节] 内。
//   - offset == 已收字节：纯追加；
//   - offset < 已收字节：若重叠区字节与已收内容一致，视为重传——完全在已收范围内的分块直接返回
//     replayed=true（幂等，丢了 ACK 也不会写坏文件）；跨越已收端点则只追加未收部分；
//   - 重叠区不一致 / offset > 已收字节（跳空）：E_CONFLICT，并提示用 file.uploadStatus 查已收字节。
func callUploadChunk(raw json.RawMessage, root string) (json.RawMessage, *pluginapi.Error) {
	var p uploadChunkParams
	if perr := decodeParams(raw, &p); perr != nil {
		return nil, perr
	}
	if p.Path == "" {
		return nil, invalid("path 不能为空")
	}
	if p.Offset < 0 {
		return nil, invalid("offset 不能为负数")
	}
	if p.TotalSize != nil && *p.TotalSize < 0 {
		return nil, invalid("total_size 不能为负数")
	}
	abs, perr := ResolveTarget(root, p.Path)
	if perr != nil {
		return nil, perr
	}
	parent, err := os.Stat(filepath.Dir(abs))
	if err != nil || !parent.IsDir() {
		return nil, invalid("目标目录不存在")
	}
	data, perr := decodeBase64Payload(p.DataBase64)
	if perr != nil {
		return nil, perr
	}
	if len(data) == 0 {
		return nil, invalid("data_base64 不能为空")
	}
	if p.TotalSize != nil && p.Offset+int64(len(data)) > *p.TotalSize {
		return nil, invalid("分块越过 total_size 边界")
	}
	size, _, perr := probeSize(abs)
	if perr != nil {
		return nil, perr
	}
	if p.Offset > size {
		return nil, conflict(fmt.Sprintf("offset=%d 超过已收字节 %d（先调 file.uploadStatus 断点续传）", p.Offset, size))
	}
	replayed := false
	writeFrom := 0
	if p.Offset < size {
		overlap := len(data)
		if remain := size - p.Offset; int64(overlap) > remain {
			overlap = int(remain)
		}
		existing, perr := readAt(abs, p.Offset, overlap)
		if perr != nil {
			return nil, perr
		}
		if !bytes.Equal(existing, data[:overlap]) {
			return nil, conflict("offset 处内容与已收字节不一致（同名文件被其他写入覆盖？）")
		}
		writeFrom = overlap
		if p.Offset+int64(len(data)) <= size {
			replayed = true
		}
	}
	received := size
	if !replayed {
		if err := writeAt(abs, data[writeFrom:], p.Offset+int64(writeFrom)); err != nil {
			return nil, classify(err)
		}
		received = p.Offset + int64(len(data))
	}
	result := UploadChunkResult{Path: displayPath(p.Path), Received: received, TotalSize: p.TotalSize}
	result.Complete = p.TotalSize != nil && received == *p.TotalSize
	result.Replayed = replayed
	return encode(result)
}

// callUploadStatus 处理 file.uploadStatus：返回目标文件已收字节数（不存在按 0，客户端据此续传）。
func callUploadStatus(raw json.RawMessage, root string) (json.RawMessage, *pluginapi.Error) {
	var p uploadStatusParams
	if perr := decodeParams(raw, &p); perr != nil {
		return nil, perr
	}
	if p.Path == "" {
		return nil, invalid("path 不能为空")
	}
	abs, perr := ResolveTarget(root, p.Path)
	if perr != nil {
		return nil, perr
	}
	size, exists, perr := probeSize(abs)
	if perr != nil {
		return nil, perr
	}
	return encode(UploadStatusResult{Path: displayPath(p.Path), Received: size, Exists: exists})
}

// probeSize 返回已收字节与是否存在；目录按参数错误拒绝，避免"续传"写到目录上。
func probeSize(abs string) (int64, bool, *pluginapi.Error) {
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, false, nil
		}
		return 0, false, classify(err)
	}
	if info.IsDir() {
		return 0, true, invalid("目标路径是目录，不能作为上传目标")
	}
	return info.Size(), true, nil
}

// readAt 读目标文件 [off, off+n) 与重传分块比对，用于判定"重复上传同 offset"。
func readAt(abs string, off int64, n int) ([]byte, *pluginapi.Error) {
	file, err := os.Open(abs)
	if err != nil {
		return nil, classify(err)
	}
	defer func() { _ = file.Close() }()
	buf := make([]byte, n)
	if _, err := io.ReadFull(io.NewSectionReader(file, off, int64(n)), buf); err != nil {
		return nil, classify(err)
	}
	return buf, nil
}

// writeAt 在 off 处写入 p；O_CREATE 让首块创建文件，不截断已有内容。
func writeAt(abs string, p []byte, off int64) error {
	file, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.WriteAt(p, off); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
