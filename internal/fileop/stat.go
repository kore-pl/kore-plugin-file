package fileop

import (
	"encoding/json"
	"time"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// callStat 处理 file.stat：返回 size / mode / owner / mod_time / is_dir / is_symlink。
func callStat(raw json.RawMessage, root string, exec Executor) (json.RawMessage, *pluginapi.Error) {
	var p statParams
	if perr := decodeParams(raw, &p); perr != nil {
		return nil, perr
	}
	abs, perr := ResolveEntry(root, p.Path)
	if perr != nil {
		return nil, perr
	}
	data, err := exec.Stat(abs)
	if err != nil {
		return nil, classify(err)
	}
	return encode(StatResult{
		Path:      displayPath(p.Path),
		Size:      data.Size,
		Mode:      modeString(uint32(data.Mode.Perm())),
		Owner:     Owner{UID: data.UID, GID: data.GID},
		ModTime:   data.ModTime.Format(time.RFC3339),
		IsDir:     data.IsDir,
		IsSymlink: data.IsSymlink,
	})
}
