package fileop

import (
	"path"
	"strings"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

const (
	formatZip   = "zip"
	formatTar   = "tar"
	formatTarGz = "tar.gz"

	// maxEntryNameLen 与 backup 的 validateRestoreHeader 对齐：4096 字节。
	maxEntryNameLen = 4096
	// maxArchiveEntries 是条目数上限，避免解压一个超大目录把插件拖死。
	maxArchiveEntries = 100000
	// maxArchiveEntryBytes 是单条目上限：2 GiB（与 backup 一致）。
	maxArchiveEntryBytes int64 = 2 << 30
	// maxLinkTargetLen 是符号链接目标长度上限。
	maxLinkTargetLen = 4096
)

// detectArchiveFormat 只按扩展名识别格式，避免把任意文件当归档解析。
func detectArchiveFormat(name string) (string, *pluginapi.Error) {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return formatTarGz, nil
	case strings.HasSuffix(lower, ".tar"):
		return formatTar, nil
	case strings.HasSuffix(lower, ".zip"):
		return formatZip, nil
	}
	return "", invalid("不支持的归档格式：仅支持 .zip / .tar / .tar.gz / .tgz")
}

// parseCompressFormat 解析 file.compress 的 format 参数，缺省 zip。
func parseCompressFormat(raw string) (string, *pluginapi.Error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "zip":
		return formatZip, nil
	case "tar.gz", "tgz":
		return formatTarGz, nil
	case "tar":
		return formatTar, nil
	}
	return "", invalid("format 只支持 zip / tar / tar.gz（缺省 zip）")
}

// validateEntryName 六层拒绝归档条目名：空、超长、NUL/反斜杠、绝对路径、
// .. 段、Clean 后仍非法；整包中止而不是跳过单条。
func validateEntryName(raw string) (string, *pluginapi.Error) {
	if raw == "" {
		return "", invalid("归档条目路径为空，整包拒绝")
	}
	if len(raw) > maxEntryNameLen {
		return "", invalid("归档条目路径超过 4096 字节上限，整包拒绝")
	}
	if strings.ContainsRune(raw, 0) || strings.Contains(raw, "\\") {
		return "", invalid("归档条目路径含非法字符，整包拒绝")
	}
	if path.IsAbs(raw) {
		return "", invalid("归档条目使用绝对路径，整包拒绝")
	}
	for _, segment := range strings.Split(raw, "/") {
		if segment == ".." {
			return "", invalid("归档条目含 .. 段，整包拒绝")
		}
	}
	clean := path.Clean(raw)
	if clean == "." || clean == "" || strings.HasPrefix(clean, "../") {
		return "", invalid("归档条目路径非法，整包拒绝")
	}
	return clean, nil
}

// validateLinkTarget 拒绝绝对目标与逃出解压目录的相对目标。
func validateLinkTarget(entryName, target string) *pluginapi.Error {
	if target == "" || len(target) > maxLinkTargetLen {
		return invalid("符号链接目标为空或超长，整包拒绝")
	}
	if strings.ContainsRune(target, 0) || strings.Contains(target, "\\") {
		return invalid("符号链接目标含非法字符，整包拒绝")
	}
	if path.IsAbs(target) {
		return invalid("符号链接目标为绝对路径，整包拒绝")
	}
	resolved := path.Clean(path.Join(path.Dir(entryName), target))
	if resolved == ".." || strings.HasPrefix(resolved, "../") {
		return invalid("符号链接目标越出目标目录，整包拒绝")
	}
	return nil
}

// entryAbsPath 让归档条目复用与其它方法完全相同的 ResolveTarget：
// 词法层级 + 最近已存在祖先的符号链接解析都在同一条路径上完成。
func entryAbsPath(root, destRel, name string) (string, *pluginapi.Error) {
	return ResolveTarget(root, path.Join(destRel, name))
}

// opErrorFromProtocol 把路径校验的协议错误转回执行面错误，保留 E_INVALID 语义。
func opErrorFromProtocol(perr *pluginapi.Error) *OpError {
	if perr.Code == pluginapi.E_INVALID {
		return newOpError(ErrInvalid, perr.Message)
	}
	return newOpError(ErrInternal, perr.Message)
}
