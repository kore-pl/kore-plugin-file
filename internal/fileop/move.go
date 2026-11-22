package fileop

import (
	"encoding/json"
	"path"
	"path/filepath"
	"strings"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// moveParams 是 file.move 的显式入参形状。
type moveParams struct {
	Src       string `json:"src"`
	Dst       string `json:"dst"`
	Overwrite bool   `json:"overwrite"`
}

// MoveResult 是 file.move 的结果；method 说明走的是 rename 还是跨文件系统回退。
type MoveResult struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Moved  bool   `json:"moved"`
	Method string `json:"method"`
}

// MoveData 是 file.move 执行面返回的结果。
type MoveData struct {
	Method string
}

// callMove 处理 file.move（同根内重命名 / 移动）：src 必须存在，dst 父目录必须存在。
func callMove(raw json.RawMessage, root string, exec Executor) (json.RawMessage, *pluginapi.Error) {
	var p moveParams
	if perr := decodeParams(raw, &p); perr != nil {
		return nil, perr
	}
	if p.Src == "" {
		return nil, invalid("src 不能为空")
	}
	if p.Dst == "" {
		return nil, invalid("dst 不能为空")
	}
	srcDisplay := displayPath(p.Src)
	if srcDisplay == "." {
		return nil, invalid("不允许移动配置根目录")
	}
	dstDisplay := displayPath(p.Dst)
	if srcDisplay == dstDisplay {
		return nil, invalid("src 与 dst 相同")
	}
	if strings.HasPrefix(dstDisplay, srcDisplay+"/") {
		return nil, invalid("不能把目录移动到自身子目录内")
	}
	srcAbs, perr := ResolveEntry(root, p.Src)
	if perr != nil {
		return nil, perr
	}
	dstAbs, perr := resolveDestination(root, dstDisplay)
	if perr != nil {
		return nil, perr
	}
	data, err := exec.Move(srcAbs, dstAbs, p.Overwrite)
	if err != nil {
		return nil, classify(err)
	}
	return encode(MoveResult{From: srcDisplay, To: dstDisplay, Moved: true, Method: data.Method})
}

// resolveDestination 只解析 dst 的父目录（必须已存在且仍在根内），
// 末段保持未解析，避免把已有符号链接替换成其目标。
func resolveDestination(root, display string) (string, *pluginapi.Error) {
	if display == "." {
		return "", invalid("目标不能是配置根目录")
	}
	base := path.Base(display)
	if base == "." || base == ".." || base == "/" {
		return "", invalid("目标路径非法")
	}
	parentAbs, perr := ResolveExisting(root, path.Dir(display))
	if perr != nil {
		return "", perr
	}
	return filepath.Join(parentAbs, base), nil
}
