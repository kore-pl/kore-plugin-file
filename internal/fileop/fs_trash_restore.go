package fileop

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// TrashRestore 把条目数据移回 destAbs：目标已存在时按 overwrite 决定覆盖或冲突，
// 成功（或失败回滚）后删除条目目录。
func (realFS) TrashRestore(trashBase, id, destAbs string, overwrite bool) (TrashEntry, error) {
	entry, err := (realFS{}).TrashGet(trashBase, id)
	if err != nil {
		return TrashEntry{}, err
	}
	entryAbs := filepath.Join(trashBase, trashEntriesDirName, id)
	dataAbs := filepath.Join(entryAbs, trashDataName)
	if err := prepareRestoreTarget(destAbs, overwrite); err != nil {
		return TrashEntry{}, err
	}
	if err := os.Rename(dataAbs, destAbs); err != nil {
		return TrashEntry{}, err
	}
	if err := os.RemoveAll(entryAbs); err != nil {
		return TrashEntry{}, err
	}
	return entry, nil
}

// prepareRestoreTarget 校验恢复目标：已存在时必须显式 overwrite，且只允许同类型、空目录。
func prepareRestoreTarget(destAbs string, overwrite bool) error {
	info, err := os.Lstat(destAbs)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !overwrite {
		return newOpError(ErrConflict, "原路径已存在，overwrite=true 可覆盖")
	}
	if info.IsDir() {
		items, err := os.ReadDir(destAbs)
		if err != nil {
			return err
		}
		if len(items) > 0 {
			return newOpError(ErrConflict, "原路径是非空目录，不能覆盖；请先移走该目录")
		}
	}
	return os.Remove(destAbs)
}

// TrashPurge 永久删除回收站条目：all=true 清空并重建条目目录，否则只删指定 id。
func (realFS) TrashPurge(trashBase, id string, all bool) (int, error) {
	dir := filepath.Join(trashBase, trashEntriesDirName)
	if all {
		items, err := os.ReadDir(dir)
		if err != nil {
			return 0, err
		}
		count := 0
		for _, item := range items {
			if item.IsDir() {
				count++
			}
		}
		if err := os.RemoveAll(dir); err != nil {
			return 0, err
		}
		if err := ensureTrashSubdir(dir); err != nil {
			return 0, err
		}
		return count, nil
	}
	entryAbs := filepath.Join(dir, id)
	if _, err := os.Lstat(entryAbs); err != nil {
		return 0, opErrFromFS(err, "回收站条目不存在")
	}
	if err := os.RemoveAll(entryAbs); err != nil {
		return 0, err
	}
	return 1, nil
}

// readTrashMeta 读取并解析 meta.json；损坏按 E_INTERNAL 如实报错，不静默当空条目。
func readTrashMeta(path string) (TrashEntry, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return TrashEntry{}, opErrFromFS(err, "回收站条目元数据缺失")
	}
	var entry TrashEntry
	if err := json.Unmarshal(payload, &entry); err != nil {
		return TrashEntry{}, newOpError(ErrInternal, "回收站条目元数据损坏: "+err.Error())
	}
	return entry, nil
}

// opErrFromFS 把 fs 错误转成带业务语义的 OpError，保留未找到与权限两类。
func opErrFromFS(err error, notFoundMessage string) error {
	if errors.Is(err, fs.ErrNotExist) {
		return newOpError(ErrNotFound, notFoundMessage)
	}
	return err
}
