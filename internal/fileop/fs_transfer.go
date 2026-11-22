package fileop

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// ErrTransferTooLarge 表示内容超过单帧可承载上限；调用面据此给出带口径的 E_INVALID。
var ErrTransferTooLarge = errors.New("传输内容超过单帧上限")

// UploadData 是 file.upload 执行面返回的写后元信息。
type UploadData struct {
	Size    int64
	SHA256  string
	Created bool
}

// DownloadData 是 file.download 执行面返回的原始内容。
type DownloadData struct {
	Size    int64
	Content []byte
}

// Upload 校验目标后原子落盘：同名且 overwrite=false 回 E_CONFLICT，
// 覆盖时继承已有文件权限位，成功后返回 size 与 sha256。
func (realFS) Upload(abs string, data []byte, overwrite bool) (UploadData, error) {
	info, err := os.Lstat(abs)
	existed := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return UploadData{}, err
	}
	if existed {
		if info.IsDir() {
			return UploadData{}, newOpError(ErrConflict, "目标位置是目录，不能上传同名文件")
		}
		if !overwrite {
			return UploadData{}, newOpError(ErrConflict, "目标文件已存在，overwrite=true 可覆盖")
		}
	}
	parent, err := os.Stat(filepath.Dir(abs))
	if err != nil {
		return UploadData{}, err
	}
	if !parent.IsDir() {
		return UploadData{}, newOpError(ErrInvalid, "上传目标目录不存在或不是目录")
	}
	perm := DefaultFileMode.Perm()
	if existed {
		perm = info.Mode().Perm()
	}
	if err := atomicWrite(abs, string(data), perm); err != nil {
		return UploadData{}, err
	}
	sum := sha256.Sum256(data)
	return UploadData{Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), Created: !existed}, nil
}

// Download 只回常规文件内容；目录、特殊文件与超限文件都拒绝，超限用 ErrTransferTooLarge 交回调用面。
func (realFS) Download(abs string, limit int64) (DownloadData, error) {
	info, err := os.Stat(abs)
	if err != nil {
		return DownloadData{}, err
	}
	if info.IsDir() {
		return DownloadData{}, newOpError(ErrInvalid, "目标是目录，请先用 file.compress 打包再下载")
	}
	if !info.Mode().IsRegular() {
		return DownloadData{}, newOpError(ErrInvalid, "目标不是常规文件")
	}
	if info.Size() > limit {
		return DownloadData{Size: info.Size()}, ErrTransferTooLarge
	}
	file, err := os.Open(abs)
	if err != nil {
		return DownloadData{}, err
	}
	defer func() { _ = file.Close() }()
	buf, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return DownloadData{}, err
	}
	if int64(len(buf)) > limit {
		return DownloadData{Size: int64(len(buf))}, ErrTransferTooLarge
	}
	return DownloadData{Size: int64(len(buf)), Content: buf}, nil
}
