package fileop

import (
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

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

// displayPath 把已校验的用户路径归一为可回显的相对路径，绝不带根目录前缀。
func displayPath(raw string) string {
	return path.Clean(raw)
}

// entryPath 拼接条目对外的相对路径。
func entryPath(dir, name string) string {
	return path.Join(dir, name)
}

// modeString 把权限位格式化为四位八进制字符串（如 "0644"）。
func modeString(mode uint32) string {
	return fmt.Sprintf("%04o", mode)
}

// parseDirMode 解析 file.mkdir 的 mode 参数（八进制字符串），缺省 0755。
func parseDirMode(raw string) (uint32, *pluginapi.Error) {
	if raw == "" {
		return uint32(DefaultDirMode.Perm()), nil
	}
	s := strings.TrimPrefix(strings.TrimPrefix(raw, "0o"), "0O")
	value, err := strconv.ParseUint(s, 8, 32)
	if err != nil || value > 0o777 {
		return 0, invalid("mode 必须是 0 到 0777 的八进制权限，例如 \"0755\"")
	}
	return uint32(value), nil
}

// checkContent 校验 file.write 的 content 字节数不超过 8 MiB。
func checkContent(content string) *pluginapi.Error {
	if len(content) > MaxWriteBytes {
		return invalid(fmt.Sprintf("content 超过 %d 字节上限", MaxWriteBytes))
	}
	return nil
}

// effectiveReadLimit 计算本次读取的有效上限：缺省 1 MiB，显式值不超过 8 MiB 硬上限。
func effectiveReadLimit(requested int64) (int64, *pluginapi.Error) {
	if requested == 0 {
		return DefaultReadLimit, nil
	}
	if requested < 1 || requested > MaxReadBytes {
		return 0, invalid(fmt.Sprintf("max_bytes 必须在 1 到 %d 之间", MaxReadBytes))
	}
	return requested, nil
}
