package fileop

import (
	"encoding/json"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// Call 执行一个 file 方法：先校验方法名与参数，再调用执行面并结构化结果。
// exec 为 nil 时使用真实文件系统（realFS）。
func Call(method string, params json.RawMessage, root string, exec Executor) (json.RawMessage, *pluginapi.Error) {
	verb, ok := businessVerb(method)
	if !ok {
		return nil, errNotFound(method)
	}
	if exec == nil {
		exec = realFS{}
	}
	switch verb {
	case "list":
		return callList(params, root, exec)
	case "read":
		return callRead(params, root, exec)
	case "write":
		return callWrite(params, root, exec)
	case "delete":
		return callDelete(params, root, exec)
	case "mkdir":
		return callMkdir(params, root, exec)
	case "stat":
		return callStat(params, root, exec)
	}
	return nil, errNotFound(method)
}

// businessVerb 把 file.<verb> 或裸 <verb> 归一为动词名。
func businessVerb(method string) (string, bool) {
	verb := method
	if len(method) > len("file.") && method[:len("file.")] == "file." {
		verb = method[len("file."):]
	}
	switch verb {
	case "list", "read", "write", "delete", "mkdir", "stat":
		return verb, true
	}
	return "", false
}
