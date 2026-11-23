package fileop

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// TrashPut 把被删除对象移入回收站：先建条目目录并原子写 meta.json，再 rename 数据；
// rename 失败时回滚整条（不留残目录）。非空目录在 recursive=false 时仍然回 E_CONFLICT，
// 保持与旧 delete 完全一致的语义。
func (realFS) TrashPut(trashBase, absSrc, originalRel string, recursive bool) (TrashEntry, error) {
	info, err := os.Lstat(absSrc)
	if err != nil {
		return TrashEntry{}, err
	}
	if info.IsDir() && !recursive {
		items, err := os.ReadDir(absSrc)
		if err != nil {
			return TrashEntry{}, err
		}
		if len(items) > 0 {
			return TrashEntry{}, newOpError(ErrConflict, "目录非空，recursive=true 可递归删除")
		}
	}
	entryAbs, entry, err := createTrashEntry(trashBase, originalRel, info)
	if err != nil {
		return TrashEntry{}, err
	}
	if err := os.Rename(absSrc, filepath.Join(entryAbs, trashDataName)); err != nil {
		_ = os.RemoveAll(entryAbs)
		return TrashEntry{}, err
	}
	return entry, nil
}

// createTrashEntry 建条目目录并写入 meta.json（0600 原子写），返回条目路径与元数据。
func createTrashEntry(trashBase, originalRel string, info fs.FileInfo) (string, TrashEntry, error) {
	entries := filepath.Join(trashBase, trashEntriesDirName)
	for attempt := 0; attempt < 3; attempt++ {
		id, err := newTrashID()
		if err != nil {
			return "", TrashEntry{}, err
		}
		entryAbs := filepath.Join(entries, id)
		if err := os.Mkdir(entryAbs, 0o700); err != nil {
			if errors.Is(err, fs.ErrExist) {
				continue
			}
			return "", TrashEntry{}, err
		}
		entry := TrashEntry{
			ID: id, Path: originalRel, Kind: trashKind(info.Mode()),
			Size: info.Size(), DeletedAt: time.Now().UTC().Format(time.RFC3339),
		}
		payload, err := json.Marshal(entry)
		if err != nil {
			_ = os.RemoveAll(entryAbs)
			return "", TrashEntry{}, err
		}
		if err := atomicWrite(filepath.Join(entryAbs, trashMetaName), string(payload), 0o600); err != nil {
			_ = os.RemoveAll(entryAbs)
			return "", TrashEntry{}, err
		}
		return entryAbs, entry, nil
	}
	return "", TrashEntry{}, newOpError(ErrConflict, "回收站条目 id 冲突，请重试")
}

// newTrashID 生成时间有序且带随机后缀的条目 id：<19 位纳秒>-<8 位十六进制>。
func newTrashID() (string, error) {
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%019d-%08x", time.Now().UnixNano(), binary.BigEndian.Uint32(suffix[:])), nil
}

// trashKind 把文件类型归一为 file / dir / symlink。
func trashKind(mode fs.FileMode) string {
	switch {
	case mode&fs.ModeSymlink != 0:
		return "symlink"
	case mode.IsDir():
		return "dir"
	default:
		return "file"
	}
}

// TrashGet 读取单个条目的元数据；条目目录或数据缺失都按 E_NOTFOUND 处理。
func (realFS) TrashGet(trashBase, id string) (TrashEntry, error) {
	entryAbs := filepath.Join(trashBase, trashEntriesDirName, id)
	if _, err := os.Lstat(filepath.Join(entryAbs, trashDataName)); err != nil {
		return TrashEntry{}, opErrFromFS(err, "回收站条目不存在")
	}
	entry, err := readTrashMeta(filepath.Join(entryAbs, trashMetaName))
	if err != nil {
		return TrashEntry{}, err
	}
	if entry.ID != id {
		return TrashEntry{}, newOpError(ErrInternal, "回收站条目元数据损坏：id 不匹配")
	}
	return entry, nil
}

// TrashList 列出回收站条目：id 倒序（时间有序），损坏条目计入 Skipped 而不让整次失败。
func (realFS) TrashList(trashBase string, limit int) (TrashListing, error) {
	dir := filepath.Join(trashBase, trashEntriesDirName)
	items, err := os.ReadDir(dir)
	if err != nil {
		return TrashListing{}, err
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if item.IsDir() && trashIDPattern.MatchString(item.Name()) {
			ids = append(ids, item.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	listing := TrashListing{Entries: []TrashEntry{}}
	for _, id := range ids {
		entryAbs := filepath.Join(dir, id)
		if _, err := os.Lstat(filepath.Join(entryAbs, trashDataName)); err != nil {
			listing.Skipped++
			continue
		}
		entry, err := readTrashMeta(filepath.Join(entryAbs, trashMetaName))
		if err != nil || entry.ID != id {
			listing.Skipped++
			continue
		}
		listing.Total++
		if len(listing.Entries) < limit {
			listing.Entries = append(listing.Entries, entry)
		}
	}
	listing.Truncated = listing.Total > len(listing.Entries)
	return listing, nil
}
