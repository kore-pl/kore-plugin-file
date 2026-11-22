package fileop

import (
	"errors"
	"io/fs"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// classify 把执行面与文件系统错误映射为协议错误码。
// 故障注入类别优先映射（四类故障各有对应码），其次业务错误，最后按 errno 归类；
// 所有 message 只描述原因与操作，不拼接宿主绝对路径。
func classify(err error) *pluginapi.Error {
	var fe *FaultError
	if errors.As(err, &fe) {
		switch fe.Kind {
		case FaultKill:
			return upstream("进程被信号 9 终止: " + fe.Message)
		case FaultTimeout:
			return upstream("调用超时: " + fe.Message)
		case FaultDisk:
			return internalErr("磁盘满: " + fe.Message)
		case FaultCgroup:
			return unsupported("cgroup 限额触发: " + fe.Message)
		}
	}
	var oe *OpError
	if errors.As(err, &oe) {
		return mapOpError(oe)
	}
	return mapFSError(err)
}

// mapOpError 把业务错误类别映射为协议错误码。
func mapOpError(oe *OpError) *pluginapi.Error {
	switch oe.Kind {
	case ErrInvalid:
		return invalid(oe.Message)
	case ErrNotFound:
		return notFound(oe.Message)
	case ErrConflict:
		return conflict(oe.Message)
	case ErrUnsupported:
		return unsupported(oe.Message)
	default:
		return internalErr(oe.Message)
	}
}

// mapFSError 按标准库哨兵错误归类，仅保留操作名（如 lstat），不带路径。
func mapFSError(err error) *pluginapi.Error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return notFound("路径不存在")
	case errors.Is(err, fs.ErrExist):
		return conflict("目标已存在")
	case errors.Is(err, fs.ErrPermission):
		return internalErr("权限不足")
	}
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return internalErr("文件系统操作失败: " + pe.Op)
	}
	return internalErr("文件系统操作失败")
}

// invalid 构造参数/策略拒绝错误（含安全校验拒绝）。
func invalid(msg string) *pluginapi.Error {
	return &pluginapi.Error{Code: pluginapi.E_INVALID, Message: msg}
}

// notFound 构造资源不存在错误。
func notFound(msg string) *pluginapi.Error {
	return &pluginapi.Error{Code: pluginapi.E_NOTFOUND, Message: msg}
}

// conflict 构造状态冲突错误（目标已存在、目录非空）。
func conflict(msg string) *pluginapi.Error {
	return &pluginapi.Error{Code: pluginapi.E_CONFLICT, Message: msg}
}

// errNotFound 构造未知方法错误。
func errNotFound(method string) *pluginapi.Error {
	return &pluginapi.Error{Code: pluginapi.E_NOTFOUND, Message: "未知方法: " + method}
}

// upstream 构造上游故障错误。
func upstream(msg string) *pluginapi.Error {
	return &pluginapi.Error{Code: pluginapi.E_UPSTREAM, Message: msg}
}

// internalErr 构造内部错误。
func internalErr(msg string) *pluginapi.Error {
	return &pluginapi.Error{Code: pluginapi.E_INTERNAL, Message: msg}
}

// unsupported 构造不支持错误。
func unsupported(msg string) *pluginapi.Error {
	return &pluginapi.Error{Code: pluginapi.E_UNSUPPORTED, Message: msg}
}
