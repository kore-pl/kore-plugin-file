package fileop

import (
	"encoding/json"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// callList 处理 file.list：校验 path 后返回目录条目列表。
func callList(raw json.RawMessage, root string, exec Executor) (json.RawMessage, *pluginapi.Error) {
	var p listParams
	if perr := decodeParams(raw, &p); perr != nil {
		return nil, perr
	}
	abs, perr := ValidatePath(root, p.Path)
	if perr != nil {
		return nil, perr
	}
	entries, err := exec.List(root, abs)
	if err != nil {
		return nil, classify(err)
	}
	if entries == nil {
		entries = []Entry{}
	}
	return encode(ListResult{Root: root, Entries: entries, Count: len(entries)})
}

// noopExecutor 是骨架默认执行面：不读写宿主文件，只返回空结果。
// 四类故障注入测试提供自己的 exec，让 Call 走错误映射路径。
type noopExecutor struct{}

func (noopExecutor) List(root, rel string) ([]Entry, error) { return nil, nil }
func (noopExecutor) Read(root, rel string) (string, error)  { return "", nil }
func (noopExecutor) Write(root, rel, content string) (bool, error) {
	return false, nil
}

// decodeParams 校验 params 是 JSON 对象并反序列化到目标形状。
func decodeParams(raw json.RawMessage, target any) *pluginapi.Error {
	if len(raw) == 0 || string(raw) == "null" {
		return invalid("缺少必填参数对象")
	}
	if raw[0] != '{' {
		return invalid("params 必须是 JSON 对象")
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return invalid("params 解析失败: " + err.Error())
	}
	return nil
}

// encode 把结果编码为 json.RawMessage；序列化失败归 E_INTERNAL。
func encode(v any) (json.RawMessage, *pluginapi.Error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, internalErr("序列化结果失败: " + err.Error())
	}
	return data, nil
}
