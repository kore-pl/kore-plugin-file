package fileop

import (
	"strings"
)

// DefaultRoot 是未配置 KORE_FILE_ROOT 时的缺省根：整机文件系统。
//
// 为什么不是某个固定子目录：插件由内核拉起时只拿到 5 个固定环境变量，
// 内核不注入 KORE_FILE_ROOT（见 kore/internal/core/supervisor/start.go 的 pluginEnv），
// 旧缺省值 /tmp/kore-file-root 在全新机器上不存在，导致 file.list 等全部
// 以「配置根目录不可访问」失败——装完就用不了。缺省取 "/" 让文件管理立即可用；
// 真正的写边界由操作系统权限与内核 system:admin 能力共同兜底。
// 需要收窄范围时显式设置 KORE_FILE_ROOT（如 /var/www），路径穿越与符号链接校验不变。
const DefaultRoot = "/"

// ResolveRoot 归一化配置的根目录：空值 / 纯空白回落 DefaultRoot，其余原样返回。
//
// 只做「缺省值收口」，不在这里做存在性校验——evalRoot 会在每次解析时
// 检查根目录存在、是目录、且是真实路径，缺省值 "/" 恒满足。
func ResolveRoot(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return DefaultRoot
	}
	return raw
}
