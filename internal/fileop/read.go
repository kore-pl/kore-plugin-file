package fileop

import (
	"encoding/json"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// callRead 处理 file.read：默认上限 1 MiB，显式 max_bytes 可调到 8 MiB；
// 超限直接拒绝并提示用 file.stat，二进制（含 NUL）只回元信息不回 content。
func callRead(raw json.RawMessage, root string, exec Executor) (json.RawMessage, *pluginapi.Error) {
	var p readParams
	if perr := decodeParams(raw, &p); perr != nil {
		return nil, perr
	}
	limit, perr := effectiveReadLimit(p.MaxBytes)
	if perr != nil {
		return nil, perr
	}
	abs, perr := ResolveExisting(root, p.Path)
	if perr != nil {
		return nil, perr
	}
	data, err := exec.Read(abs, limit)
	if err != nil {
		return nil, classify(err)
	}
	result := ReadResult{
		Path:      displayPath(p.Path),
		Size:      data.Size,
		Truncated: data.Truncated,
		IsBinary:  data.Binary,
	}
	if !data.Binary {
		content := string(data.Content)
		result.Content = &content
	}
	return encode(result)
}
