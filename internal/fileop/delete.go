package fileop

import (
	"encoding/json"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// callDelete 处理 file.delete：默认把目标移入回收站（可恢复），permanent=true 才直接删除。
//
// 语义变更（0.3.0）：旧版本无条件直接删除；现在默认走回收站，删除后可用
// file.trash.list 查看、file.trash.restore 恢复。需要旧语义时显式传 permanent=true。
// 其余约束不变：非空目录必须 recursive=true，配置根目录禁止删除。
func callDelete(raw json.RawMessage, root string, exec Executor) (json.RawMessage, *pluginapi.Error) {
	var p deleteParams
	if perr := decodeParams(raw, &p); perr != nil {
		return nil, perr
	}
	display := displayPath(p.Path)
	if display == "." {
		return nil, invalid("不允许删除配置根目录")
	}
	abs, perr := ResolveEntry(root, p.Path)
	if perr != nil {
		return nil, perr
	}
	if p.Permanent {
		if err := exec.Delete(abs, p.Recursive); err != nil {
			return nil, classify(err)
		}
		return encode(DeleteResult{Path: display, Deleted: true, Permanent: true})
	}
	base, _, perr := trashBase(root, true)
	if perr != nil {
		return nil, perr
	}
	entry, err := exec.TrashPut(base, abs, display, p.Recursive)
	if err != nil {
		return nil, classify(err)
	}
	return encode(DeleteResult{Path: display, Deleted: true, TrashID: entry.ID})
}
