package fileop

import (
	"encoding/json"
	"strings"

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
	case "upload":
		return callUpload(params, root, exec)
	case "download":
		return callDownload(params, root, exec)
	case "extract":
		return callExtract(params, root, exec)
	case "compress":
		return callCompress(params, root, exec)
	case "chmod":
		return callChmod(params, root, exec)
	case "move":
		return callMove(params, root, exec)
	case "rename":
		return callRename(params, root, exec)
	case "copy":
		return callCopy(params, root, exec)
	case "search":
		return callSearch(params, root, exec)
	case "trash.list":
		return callTrashList(params, root, exec)
	case "trash.restore":
		return callTrashRestore(params, root, exec)
	case "trash.purge":
		return callTrashPurge(params, root, exec)
	case "batch":
		return callBatch(params, root, exec)
	}
	return nil, errNotFound(method)
}

// businessVerb 把 file.<verb> 或裸 <verb> 归一为动词名。
// 裸名是历史兼容面，只保留最初的六个方法；L2 / L3 新方法必须写全 file.<verb>。
func businessVerb(method string) (string, bool) {
	verb := method
	namespaced := strings.HasPrefix(method, "file.")
	if namespaced {
		verb = method[len("file."):]
	}
	switch verb {
	case "list", "read", "write", "delete", "mkdir", "stat":
		return verb, true
	case "upload", "download", "extract", "compress", "chmod", "move",
		"rename", "copy", "search", "trash.list", "trash.restore", "trash.purge", "batch":
		return verb, namespaced
	}
	return "", false
}
