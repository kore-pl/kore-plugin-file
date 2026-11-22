package fileop

import (
	"encoding/json"
	"io/fs"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// callMkdir 处理 file.mkdir：默认 0755，目标已存在回 E_CONFLICT，父目录不存在回 E_NOTFOUND。
func callMkdir(raw json.RawMessage, root string, exec Executor) (json.RawMessage, *pluginapi.Error) {
	var p mkdirParams
	if perr := decodeParams(raw, &p); perr != nil {
		return nil, perr
	}
	mode, perr := parseDirMode(p.Mode)
	if perr != nil {
		return nil, perr
	}
	abs, perr := ResolveTarget(root, p.Path)
	if perr != nil {
		return nil, perr
	}
	if err := exec.Mkdir(abs, fs.FileMode(mode)); err != nil {
		return nil, classify(err)
	}
	return encode(MkdirResult{Path: displayPath(p.Path), Mode: modeString(mode), Created: true})
}
