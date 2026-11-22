package fileop

import (
	"encoding/json"
	"time"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// callWrite 处理 file.write：校验内容上限与路径后原子写入（临时文件 + rename）。
// 父目录不存在时按 create_if_missing 决定自动创建还是返回 E_NOTFOUND。
func callWrite(raw json.RawMessage, root string, exec Executor) (json.RawMessage, *pluginapi.Error) {
	var p writeParams
	if perr := decodeParams(raw, &p); perr != nil {
		return nil, perr
	}
	abs, perr := ResolveTarget(root, p.Path)
	if perr != nil {
		return nil, perr
	}
	if perr := checkContent(p.Content); perr != nil {
		return nil, perr
	}
	data, err := exec.Write(abs, p.Content, p.CreateIfMissing)
	if err != nil {
		return nil, classify(err)
	}
	return encode(WriteResult{
		Path:    displayPath(p.Path),
		Size:    data.Size,
		ModTime: data.ModTime.Format(time.RFC3339),
		Created: data.Created,
	})
}
