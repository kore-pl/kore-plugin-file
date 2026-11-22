package fileop

import (
	"encoding/json"
	"strings"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// callRead 处理 file.read：校验 path 后返回文本内容（骨架口径不真读宿主文件）。
func callRead(raw json.RawMessage, root string, exec Executor) (json.RawMessage, *pluginapi.Error) {
	var p readParams
	if perr := decodeParams(raw, &p); perr != nil {
		return nil, perr
	}
	abs, perr := ValidatePath(root, p.Path)
	if perr != nil {
		return nil, perr
	}
	content, err := exec.Read(root, abs)
	if err != nil {
		return nil, classify(err)
	}
	return encode(ReadResult{
		Path:      strings.TrimPrefix(abs, root+"/"),
		Content:   content,
		SizeBytes: len(content),
		Encoding:  "utf-8",
	})
}

// checkContent 校验 file.write 的 content 字段。
func checkContent(content string) *pluginapi.Error {
	if len(content) > maxContent {
		return invalid("content 超过 1 MiB 上限")
	}
	return nil
}
