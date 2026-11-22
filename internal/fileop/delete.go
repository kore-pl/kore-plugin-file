package fileop

import (
	"encoding/json"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// callDelete 处理 file.delete：删文件或空目录；非空目录且 recursive=false 回 E_CONFLICT；
// recursive=true 走 RemoveAll；配置根目录本身禁止删除。
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
	if err := exec.Delete(abs, p.Recursive); err != nil {
		return nil, classify(err)
	}
	return encode(DeleteResult{Path: display, Deleted: true})
}
