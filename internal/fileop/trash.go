package fileop

import (
	"encoding/json"
	"fmt"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// callTrashList 处理 file.trash.list：按删除时间倒序列出可恢复条目。
func callTrashList(raw json.RawMessage, root string, exec Executor) (json.RawMessage, *pluginapi.Error) {
	var p trashListParams
	if perr := decodeParams(raw, &p); perr != nil {
		return nil, perr
	}
	limit := p.Limit
	if limit == 0 {
		limit = DefaultTrashListLimit
	}
	if limit < 1 || limit > MaxTrashListLimit {
		return nil, invalid(fmt.Sprintf("limit 必须在 1 到 %d 之间", MaxTrashListLimit))
	}
	base, exists, perr := trashBase(root, false)
	if perr != nil {
		return nil, perr
	}
	if !exists {
		return encode(TrashListResult{Entries: []TrashEntry{}})
	}
	listing, err := exec.TrashList(base, limit)
	if err != nil {
		return nil, classify(err)
	}
	entries := listing.Entries
	if entries == nil {
		entries = []TrashEntry{}
	}
	return encode(TrashListResult{
		Count: len(entries), Total: listing.Total, Skipped: listing.Skipped,
		Truncated: listing.Truncated, Entries: entries,
	})
}

// callTrashRestore 处理 file.trash.restore：按 id 取回原路径，再走与写入同源的
// resolveDestination 校验父目录仍然存在且仍在根内，最后把条目移回原位。
func callTrashRestore(raw json.RawMessage, root string, exec Executor) (json.RawMessage, *pluginapi.Error) {
	var p trashRestoreParams
	if perr := decodeParams(raw, &p); perr != nil {
		return nil, perr
	}
	if perr := validateTrashID(p.ID, "id"); perr != nil {
		return nil, perr
	}
	base, _, perr := trashBase(root, true)
	if perr != nil {
		return nil, perr
	}
	entry, err := exec.TrashGet(base, p.ID)
	if err != nil {
		return nil, classify(err)
	}
	dstAbs, perr := resolveDestination(root, entry.Path)
	if perr != nil {
		return nil, perr
	}
	restored, err := exec.TrashRestore(base, p.ID, dstAbs, p.Overwrite)
	if err != nil {
		return nil, classify(err)
	}
	return encode(TrashRestoreResult{ID: restored.ID, Path: restored.Path, Restored: true})
}

// callTrashPurge 处理 file.trash.purge：按 id 永久删除一条，或 all=true 清空回收站。
// 这是不可恢复动作，必须显式给出 id 或 all=true，空参数直接拒绝。
func callTrashPurge(raw json.RawMessage, root string, exec Executor) (json.RawMessage, *pluginapi.Error) {
	var p trashPurgeParams
	if perr := decodeParams(raw, &p); perr != nil {
		return nil, perr
	}
	if p.All && p.ID != "" {
		return nil, invalid("all=true 时不要同时指定 id")
	}
	if !p.All && p.ID == "" {
		return nil, invalid("需要 id 清除单条，或 all=true 清空回收站")
	}
	if p.ID != "" {
		if perr := validateTrashID(p.ID, "id"); perr != nil {
			return nil, perr
		}
	}
	base, _, perr := trashBase(root, true)
	if perr != nil {
		return nil, perr
	}
	purged, err := exec.TrashPurge(base, p.ID, p.All)
	if err != nil {
		return nil, classify(err)
	}
	return encode(TrashPurgeResult{Purged: purged, All: p.All})
}

// validateTrashID 把 id 限制在安全字符集内，杜绝借 id 做路径穿越。
func validateTrashID(id, field string) *pluginapi.Error {
	if id == "" {
		return invalid(field + " 不能为空")
	}
	if !trashIDPattern.MatchString(id) {
		return invalid(field + " 格式非法：应为 <19 位纳秒>-<8 位十六进制>")
	}
	return nil
}
