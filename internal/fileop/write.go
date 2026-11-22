package fileop

import (
	"encoding/json"
	"strings"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// callWrite 处理 file.write：校验 path 与 content 后返回写入结果（骨架口径不真写宿主文件）。
func callWrite(raw json.RawMessage, root string, exec Executor) (json.RawMessage, *pluginapi.Error) {
	var p writeParams
	if perr := decodeParams(raw, &p); perr != nil {
		return nil, perr
	}
	abs, perr := ValidatePath(root, p.Path)
	if perr != nil {
		return nil, perr
	}
	if perr := checkContent(p.Content); perr != nil {
		return nil, perr
	}
	created, err := exec.Write(root, abs, p.Content)
	if err != nil {
		return nil, classify(err)
	}
	return encode(WriteResult{
		Path:         strings.TrimPrefix(abs, root+"/"),
		BytesWritten: len(p.Content),
		Created:      created,
		Applied:      true,
	})
}
