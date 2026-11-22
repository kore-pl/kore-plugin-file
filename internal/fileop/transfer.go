package fileop

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

const (
	// MaxTransferBytes 是 upload / download 的方法级单文件契约上限：32 MiB。
	MaxTransferBytes int64 = 32 << 20
	// frameOverheadReserve 给 JSON 信封与路径字段预留的帧内余量（64 KiB）。
	frameOverheadReserve = 64 << 10
	// MaxInlineTransferBytes 是单帧实际可承载的文件字节数。
	// 协议单帧硬上限是 pluginapi.MaxFrameSize，base64 编码膨胀 4/3，
	// 因此取 (帧上限 - 余量) / 4 * 3；超过它必须走 L3 分片，不能假装 32 MiB 走得通。
	MaxInlineTransferBytes int64 = (pluginapi.MaxFrameSize - frameOverheadReserve) / 4 * 3
)

// uploadParams 是 file.upload 的显式入参形状。
type uploadParams struct {
	Path       string `json:"path"`
	Filename   string `json:"filename"`
	DataBase64 string `json:"data_base64"`
	Overwrite  bool   `json:"overwrite"`
}

// UploadResult 是 file.upload 的结果。
type UploadResult struct {
	Path     string `json:"path"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
	Created  bool   `json:"created"`
}

// downloadParams 是 file.download 的显式入参形状。
type downloadParams struct {
	Path string `json:"path"`
}

// DownloadResult 是 file.download 的结果。
type DownloadResult struct {
	Filename   string `json:"filename"`
	SizeBytes  int64  `json:"size_bytes"`
	DataBase64 string `json:"data_base64"`
}

// callUpload 处理 file.upload：path 是目标目录，filename 是单段文件名；
// 目标路径仍走 ResolveExisting（目录）+ ResolveTarget（最终文件）双重校验。
func callUpload(raw json.RawMessage, root string, exec Executor) (json.RawMessage, *pluginapi.Error) {
	var p uploadParams
	if perr := decodeParams(raw, &p); perr != nil {
		return nil, perr
	}
	if p.Path == "" {
		return nil, invalid("path 不能为空，根目录请传 \".\"")
	}
	if perr := validateUploadName(p.Filename); perr != nil {
		return nil, perr
	}
	dirDisplay := displayPath(p.Path)
	if _, perr := ResolveExisting(root, p.Path); perr != nil {
		return nil, perr
	}
	rel := path.Join(dirDisplay, p.Filename)
	abs, perr := ResolveTarget(root, rel)
	if perr != nil {
		return nil, perr
	}
	data, perr := decodeBase64Payload(p.DataBase64)
	if perr != nil {
		return nil, perr
	}
	out, err := exec.Upload(abs, data, p.Overwrite)
	if err != nil {
		return nil, classify(err)
	}
	return encode(UploadResult{
		Path:     rel,
		Filename: p.Filename,
		Size:     out.Size,
		SHA256:   out.SHA256,
		Created:  out.Created,
	})
}

// callDownload 处理 file.download：返回 base64 内容；超单帧上限时明确提示打包下载。
func callDownload(raw json.RawMessage, root string, exec Executor) (json.RawMessage, *pluginapi.Error) {
	var p downloadParams
	if perr := decodeParams(raw, &p); perr != nil {
		return nil, perr
	}
	display := displayPath(p.Path)
	abs, perr := ResolveExisting(root, p.Path)
	if perr != nil {
		return nil, perr
	}
	out, err := exec.Download(abs, MaxInlineTransferBytes)
	if err != nil {
		if errors.Is(err, ErrTransferTooLarge) {
			return nil, downloadTooLarge(out.Size)
		}
		return nil, classify(err)
	}
	return encode(DownloadResult{
		Filename:   path.Base(display),
		SizeBytes:  out.Size,
		DataBase64: base64.StdEncoding.EncodeToString(out.Content),
	})
}

// validateUploadName 限制 filename 为单段常规文件名，禁止任何路径语义。
func validateUploadName(name string) *pluginapi.Error {
	switch {
	case name == "":
		return invalid("filename 不能为空")
	case name == "." || name == "..":
		return invalid("filename 不能是 . 或 ..")
	case strings.ContainsAny(name, "/\\"):
		return invalid("filename 不能包含路径分隔符")
	case strings.ContainsRune(name, 0):
		return invalid("filename 含 NUL 字节")
	case len(name) > 255:
		return invalid("filename 超过 255 字节上限")
	}
	return nil
}

// decodeBase64Payload 在解码前先按编码长度挡住超限请求，避免无谓的大块分配。
func decodeBase64Payload(encoded string) ([]byte, *pluginapi.Error) {
	encodedLimit := base64.StdEncoding.EncodedLen(int(MaxInlineTransferBytes))
	if len(encoded) > encodedLimit {
		return nil, uploadTooLarge(int64(len(encoded)) / 4 * 3)
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, invalid("data_base64 不是合法 base64: " + err.Error())
	}
	if int64(len(data)) > MaxInlineTransferBytes {
		return nil, uploadTooLarge(int64(len(data)))
	}
	return data, nil
}

// uploadTooLarge 说清三层口径：单帧上限、base64 膨胀、方法契约上限与 L3 分片。
func uploadTooLarge(size int64) *pluginapi.Error {
	return invalid(fmt.Sprintf(
		"文件大小约 %d 字节超过单次上传上限 %d 字节：协议单帧上限 %d 字节、base64 膨胀 4/3；方法契约上限 %d 字节，大文件分片上传在 L3 提供",
		size, MaxInlineTransferBytes, pluginapi.MaxFrameSize, MaxTransferBytes))
}

// downloadTooLarge 保留「请用目录打包下载」的可用指引，并给出实际口径。
func downloadTooLarge(size int64) *pluginapi.Error {
	return invalid(fmt.Sprintf(
		"文件大小 %d 字节超过下载上限 %d 字节（协议单帧上限 %d 字节）：文件过大，请用目录打包下载（file.compress 打包后体积可能更小；分片下载在 L3 提供）",
		size, MaxInlineTransferBytes, pluginapi.MaxFrameSize))
}
