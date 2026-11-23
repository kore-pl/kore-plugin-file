package fileop

import (
	"encoding/json"
	"path"
	"path/filepath"
	"strings"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// renameParams 是 file.rename 的显式入参形状：name 是单段新文件名，不含任何目录语义。
type renameParams struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Overwrite bool   `json:"overwrite"`
}

// RenameResult 是 file.rename 的结果；method 说明底层走的是 rename 还是跨设备回退。
type RenameResult struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Renamed bool   `json:"renamed"`
	Method  string `json:"method"`
}

// callRename 处理 file.rename（同目录改名）：与 file.move 的区别是**不能换目录**——
// 目标由「源所在目录 + 单段新名」推导，调用方没有传目录的入口。
// 冲突策略与 file.move 一致：目标已存在且 overwrite=false 回 E_CONFLICT。
func callRename(raw json.RawMessage, root string, exec Executor) (json.RawMessage, *pluginapi.Error) {
	var p renameParams
	if perr := decodeParams(raw, &p); perr != nil {
		return nil, perr
	}
	if p.Path == "" {
		return nil, invalid("path 不能为空")
	}
	if perr := validateRenameName(p.Name); perr != nil {
		return nil, perr
	}
	from := displayPath(p.Path)
	if from == "." {
		return nil, invalid("不允许重命名配置根目录")
	}
	to := path.Join(path.Dir(from), p.Name)
	if to == from {
		return nil, invalid("新名称与原名称相同")
	}
	if _, perr := ValidatePath(root, to); perr != nil {
		return nil, perr
	}
	srcAbs, perr := ResolveEntry(root, p.Path)
	if perr != nil {
		return nil, perr
	}
	dstAbs := filepath.Join(filepath.Dir(srcAbs), p.Name)
	data, err := exec.Move(srcAbs, dstAbs, p.Overwrite)
	if err != nil {
		return nil, classify(err)
	}
	return encode(RenameResult{From: from, To: to, Renamed: true, Method: data.Method})
}

// validateRenameName 只接受单段常规文件名：禁止路径分隔符，杜绝借 rename 跨目录移动。
func validateRenameName(name string) *pluginapi.Error {
	switch {
	case name == "":
		return invalid("name 不能为空")
	case name == "." || name == "..":
		return invalid("name 不能是 . 或 ..")
	case name == TrashDirName:
		return invalid("name 不能使用保留目录名 " + TrashDirName)
	case strings.ContainsAny(name, "/\\"):
		return invalid("name 不能包含路径分隔符，跨目录移动请用 file.move")
	case strings.ContainsRune(name, 0):
		return invalid("name 含 NUL 字节")
	case len(name) > 255:
		return invalid("name 超过 255 字节上限")
	}
	return nil
}
