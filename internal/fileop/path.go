package fileop

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// ValidatePath 把用户路径限制在配置根目录内，返回归一化后的绝对路径。
// 拒绝：空路径、NUL、反斜杠、绝对路径、".." 段、保留的回收站目录、以及 Clean + Rel 后越出根目录。
// 只做词法校验，不访问文件系统；符号链接逃逸由 Resolve* 二次校验兜住。
func ValidatePath(root, raw string) (string, *pluginapi.Error) {
	if raw == "" {
		return "", invalid("路径不能为空")
	}
	if strings.ContainsRune(raw, 0) {
		return "", invalid("路径含 NUL 字节")
	}
	if strings.Contains(raw, "\\") {
		return "", invalid("路径含反斜杠")
	}
	if filepath.IsAbs(raw) {
		return "", invalid("路径必须是根目录内的相对路径")
	}
	for _, seg := range strings.Split(raw, "/") {
		if seg == ".." {
			return "", invalid("路径含 .. 段，禁止穿越")
		}
	}
	if isTrashPath(raw) {
		return "", invalid("回收站目录 " + TrashDirName + " 仅通过 file.trash.* 方法访问")
	}
	rootClean := filepath.Clean(root)
	if rootClean == "" || !filepath.IsAbs(rootClean) {
		return "", internalErr("配置根目录无效")
	}
	joined := filepath.Join(rootClean, raw)
	rel, err := filepath.Rel(rootClean, joined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", invalid("路径越出配置根目录")
	}
	return joined, nil
}

// isTrashPath 判定路径的首段是否命中保留的回收站目录（已经在前面排除 ".." 段）。
func isTrashPath(raw string) bool {
	cleaned := path.Clean(raw)
	if cleaned == "." {
		return false
	}
	first := cleaned
	if index := strings.Index(cleaned, "/"); index >= 0 {
		first = cleaned[:index]
	}
	return first == TrashDirName
}

// evalRoot 解析配置根目录的真实路径（跟随根自身的符号链接）。
func evalRoot(root string) (string, *pluginapi.Error) {
	clean := filepath.Clean(root)
	if clean == "" || !filepath.IsAbs(clean) {
		return "", internalErr("配置根目录无效")
	}
	real, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return "", internalErr("配置根目录不可访问")
	}
	info, err := os.Stat(real)
	if err != nil || !info.IsDir() {
		return "", internalErr("配置根目录不可用")
	}
	return real, nil
}

// within 判定 p 是否等于 root 或位于 root 之下（基于 filepath.Rel，避免前缀误判）。
func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// ResolveExisting 校验并解析一个必须已存在的目标：跟随符号链接后二次校验根目录归属。
func ResolveExisting(root, raw string) (string, *pluginapi.Error) {
	joined, perr := ValidatePath(root, raw)
	if perr != nil {
		return "", perr
	}
	realRoot, perr := evalRoot(root)
	if perr != nil {
		return "", perr
	}
	real, err := filepath.EvalSymlinks(joined)
	if err != nil {
		return "", classify(err)
	}
	if !within(realRoot, real) {
		return "", invalid("路径经符号链接解析后越出配置根目录")
	}
	return real, nil
}

// ResolveTarget 校验可能尚不存在的目标：解析最近的已存在祖先并校验，
// 再把尚未创建的尾段拼回去；祖先链上任何符号链接逃逸都会被拒绝。
func ResolveTarget(root, raw string) (string, *pluginapi.Error) {
	joined, perr := ValidatePath(root, raw)
	if perr != nil {
		return "", perr
	}
	realRoot, perr := evalRoot(root)
	if perr != nil {
		return "", perr
	}
	cur, tail := joined, ""
	for {
		real, err := filepath.EvalSymlinks(cur)
		if err == nil {
			if !within(realRoot, real) {
				return "", invalid("路径经符号链接解析后越出配置根目录")
			}
			return filepath.Join(real, tail), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", classify(err)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", invalid("路径不存在")
		}
		tail = filepath.Join(filepath.Base(cur), tail)
		cur = parent
	}
}

// ResolveEntry 是删除与 stat 的入口语义：校验符号链接解析后的归属，但返回未解析的路径。
// 删除符号链接时只删链接本身；stat 需要看到 is_symlink，因此不能把最后一跳解析掉。
func ResolveEntry(root, raw string) (string, *pluginapi.Error) {
	joined, perr := ValidatePath(root, raw)
	if perr != nil {
		return "", perr
	}
	realRoot, perr := evalRoot(root)
	if perr != nil {
		return "", perr
	}
	real, err := filepath.EvalSymlinks(joined)
	if err != nil {
		return "", classify(err)
	}
	if !within(realRoot, real) {
		return "", invalid("路径经符号链接解析后越出配置根目录")
	}
	return joined, nil
}
