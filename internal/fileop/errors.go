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

// mapFSError 按标准库哨兵错误归类。消息带 errno/底层 os error 上下文与「怎么办」建议，
// 但绝不拼接 fs.PathError.Path（宿主绝对路径会泄露目录布局）。
func mapFSError(err error) *pluginapi.Error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return notFound("路径不存在")
	case errors.Is(err, fs.ErrExist):
		return conflict("目标已存在")
	case errors.Is(err, fs.ErrPermission):
		// 保留 E_INTERNAL（不改 E_UNAUTHORIZED）：协议 §9 的 E_UNAUTHORIZED 是 401 鉴权语义
		// （未登录 / session 失效 / capability 不足 / 签名或来源不符），处置是「重新登录或申请授权」；
		// 而这里是插件进程对宿主文件系统拿到的 POSIX EACCES，重新登录不会让权限变好。
		// 协议没有面向 OS 文件权限的专用码，故按「插件执行面失败」归 E_INTERNAL，message 写明「权限不足」+ errno + 怎么办。
		return internalErr("权限不足（" + errnoText(err) + "）；请检查文件/目录属主与权限，或以 root 运行")
	}
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return internalErr(fsFailureMessage(pe.Op, pe.Err))
	}
	return internalErr(fsFailureMessage("", err))
}

// errnoText 提取底层 errno 文案，剥掉 fs.PathError 自带的宿主绝对路径，避免泄露目录布局。
func errnoText(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) && pe.Err != nil {
		return pe.Err.Error()
	}
	if err == nil {
		return ""
	}
	return err.Error()
}

// fsFailureMessage 拼出「人话 + errno 上下文 + 怎么办」的文件系统错误，不拼接路径。
func fsFailureMessage(op string, cause error) string {
	msg := "文件系统操作失败"
	if op != "" {
		msg += "（" + op + "）"
	}
	if cause != nil {
		msg += ": " + cause.Error()
	}
	return msg + "；请检查磁盘剩余空间与目标路径的访问权限，必要时以有权限的用户运行"
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
