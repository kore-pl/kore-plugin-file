package fileop

import (
	"encoding/json"
	"time"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// callList 处理 file.list：校验路径后返回真实目录条目。
func callList(raw json.RawMessage, root string, exec Executor) (json.RawMessage, *pluginapi.Error) {
	var p listParams
	if perr := decodeParams(raw, &p); perr != nil {
		return nil, perr
	}
	abs, perr := ResolveExisting(root, p.Path)
	if perr != nil {
		return nil, perr
	}
	entries, err := exec.List(abs)
	if err != nil {
		return nil, classify(err)
	}
	display := displayPath(p.Path)
	out := make([]ListEntry, 0, len(entries))
	for _, e := range entries {
		// 回收站是插件内部存储：只在根目录层隐藏，普通方法也无法访问它。
		if display == "." && e.Name == TrashDirName {
			continue
		}
		out = append(out, ListEntry{
			Name:      e.Name,
			Path:      entryPath(display, e.Name),
			IsDir:     e.IsDir,
			Size:      e.Size,
			Mode:      modeString(uint32(e.Mode.Perm())),
			ModTime:   e.ModTime.Format(time.RFC3339),
			IsSymlink: e.IsSymlink,
		})
	}
	return encode(ListResult{Path: display, Entries: out, Count: len(out)})
}
