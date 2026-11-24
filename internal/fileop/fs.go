package fileop

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
)

// realFS 是 Executor 的真实文件系统实现；只接收已通过路径校验的绝对路径。
type realFS struct{}

// List 读取目录条目；不跟随条目符号链接，只返回 lstat 元信息。
func (realFS) List(abs string) ([]Entry, error) {
	file, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, newOpError(ErrInvalid, "目标不是目录，请用 file.stat 查看")
	}
	items, err := file.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(items))
	for _, item := range items {
		itemInfo, err := item.Info()
		if err != nil {
			continue // 条目在读取期间被删除：跳过，不让整个列表失败
		}
		uid, gid := fileOwner(itemInfo)
		entries = append(entries, Entry{
			Name:      item.Name(),
			IsDir:     itemInfo.IsDir(),
			Size:      itemInfo.Size(),
			Mode:      itemInfo.Mode(),
			ModTime:   itemInfo.ModTime(),
			IsSymlink: itemInfo.Mode()&fs.ModeSymlink != 0,
			UID:       uid,
			GID:       gid,
		})
	}
	return entries, nil
}

// Read 读取常规文件；超过有效上限直接拒绝（提示 file.stat），含 NUL 判为二进制不返回内容。
func (realFS) Read(abs string, limit int64) (ReadData, error) {
	file, err := os.Open(abs)
	if err != nil {
		return ReadData{}, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return ReadData{}, err
	}
	if info.IsDir() {
		return ReadData{}, newOpError(ErrInvalid, "目标是目录，请用 file.list 查看")
	}
	if !info.Mode().IsRegular() {
		return ReadData{}, newOpError(ErrInvalid, "目标不是常规文件，请用 file.stat 查看")
	}
	if info.Size() > limit {
		return ReadData{}, overLimitError(info.Size(), limit)
	}
	buf, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return ReadData{}, err
	}
	size := int64(len(buf))
	if size > limit {
		return ReadData{}, overLimitError(size, limit)
	}
	if bytes.IndexByte(buf, 0) >= 0 {
		return ReadData{Size: size, Binary: true}, nil
	}
	return ReadData{Content: buf, Size: size}, nil
}

// Stat 返回详细元信息；符号链接在入口处已解析，is_symlink 用 lstat 判定。
func (realFS) Stat(abs string) (StatData, error) {
	linkInfo, err := os.Lstat(abs)
	if err != nil {
		return StatData{}, err
	}
	isSymlink := linkInfo.Mode()&fs.ModeSymlink != 0
	info := linkInfo
	if isSymlink {
		if info, err = os.Stat(abs); err != nil {
			return StatData{}, err
		}
	}
	uid, gid := fileOwner(info)
	return StatData{
		Size:      info.Size(),
		Mode:      info.Mode(),
		ModTime:   info.ModTime(),
		IsDir:     info.IsDir(),
		IsSymlink: isSymlink,
		UID:       uid,
		GID:       gid,
	}, nil
}

// overLimitError 生成统一的超限拒绝错误（只含字节数，不含路径）。
func overLimitError(size, limit int64) *OpError {
	return newOpError(ErrInvalid, fmt.Sprintf("文件大小 %d 字节超过读取上限 %d 字节，请用 file.stat 查看详情或提高 max_bytes", size, limit))
}
