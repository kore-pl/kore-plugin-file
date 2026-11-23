package fileop

import (
	"encoding/json"
	"strings"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// copyParams 是 file.copy 的显式入参形状。
type copyParams struct {
	Src       string `json:"src"`
	Dst       string `json:"dst"`
	Overwrite bool   `json:"overwrite"`
}

// CopyResult 是 file.copy 的结果；kind 为 file / dir / symlink，逐项统计条目与字节。
type CopyResult struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Copied  bool   `json:"copied"`
	Kind    string `json:"kind"`
	Entries int    `json:"entries"`
	Bytes   int64  `json:"bytes"`
}

// CopyData 是 file.copy 执行面返回的统计。
type CopyData struct {
	Kind    string
	Entries int
	Bytes   int64
}

// callCopy 处理 file.copy：src 必须存在，dst 父目录必须存在。
// 覆盖策略显式：目标已存在且 overwrite=false 回 E_CONFLICT；overwrite=true 仅允许
// 同类型覆盖（文件覆盖文件、空目录覆盖空目录），非空目录不合并、直接 E_CONFLICT。
func callCopy(raw json.RawMessage, root string, exec Executor) (json.RawMessage, *pluginapi.Error) {
	var p copyParams
	if perr := decodeParams(raw, &p); perr != nil {
		return nil, perr
	}
	if p.Src == "" {
		return nil, invalid("src 不能为空")
	}
	if p.Dst == "" {
		return nil, invalid("dst 不能为空")
	}
	from := displayPath(p.Src)
	if from == "." {
		return nil, invalid("不允许复制配置根目录")
	}
	to := displayPath(p.Dst)
	if from == to {
		return nil, invalid("src 与 dst 相同")
	}
	if strings.HasPrefix(to, from+"/") {
		return nil, invalid("不能把目录复制到自身子目录内")
	}
	srcAbs, perr := ResolveEntry(root, p.Src)
	if perr != nil {
		return nil, perr
	}
	dstAbs, perr := resolveDestination(root, to)
	if perr != nil {
		return nil, perr
	}
	data, err := exec.Copy(srcAbs, dstAbs, p.Overwrite)
	if err != nil {
		return nil, classify(err)
	}
	return encode(CopyResult{
		From: from, To: to, Copied: true,
		Kind: data.Kind, Entries: data.Entries, Bytes: data.Bytes,
	})
}
