package fileop

import (
	"encoding/json"
	"fmt"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// batchParams 是 file.batch 的显式入参形状：一次请求只做同一种 op。
type batchParams struct {
	Op    string      `json:"op"`
	Items []batchItem `json:"items"`
}

// batchItem 是单条批量条目；按 op 取用对应字段（delete 用 path，move/copy 用 src+dst）。
type batchItem struct {
	Path      string `json:"path"`
	Src       string `json:"src"`
	Dst       string `json:"dst"`
	Recursive bool   `json:"recursive"`
	Permanent bool   `json:"permanent"`
	Overwrite bool   `json:"overwrite"`
}

// BatchError 是单条目失败的错误体（只含协议码与人话，不含宿主路径）。
type BatchError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// BatchItemResult 是批量操作的单条目结果：ok=false 时 error 给出逐项原因。
type BatchItemResult struct {
	Index   int         `json:"index"`
	Op      string      `json:"op"`
	Path    string      `json:"path"`
	From    string      `json:"from"`
	To      string      `json:"to"`
	OK      bool        `json:"ok"`
	TrashID string      `json:"trash_id,omitempty"`
	Error   *BatchError `json:"error,omitempty"`
}

// BatchResult 是 file.batch 的结果：部分失败不影响其他条目，逐项交代。
type BatchResult struct {
	Op        string            `json:"op"`
	Total     int               `json:"total"`
	Succeeded int               `json:"succeeded"`
	Failed    int               `json:"failed"`
	Items     []BatchItemResult `json:"items"`
}

// callBatch 处理 file.batch：对多条路径批量执行 delete / move / copy。
// 部分失败语义：单条失败只记入该条结果，其余条目继续执行；整体仍返回成功帧。
// 只有结构性错误（op 未知、items 为空或超过上限）才让整次调用失败。
func callBatch(raw json.RawMessage, root string, exec Executor) (json.RawMessage, *pluginapi.Error) {
	var p batchParams
	if perr := decodeParams(raw, &p); perr != nil {
		return nil, perr
	}
	if perr := validateBatchOp(p.Op); perr != nil {
		return nil, perr
	}
	if len(p.Items) == 0 {
		return nil, invalid("items 不能为空")
	}
	if len(p.Items) > MaxBatchItems {
		return nil, invalid(fmt.Sprintf("items 最多 %d 条，当前 %d", MaxBatchItems, len(p.Items)))
	}
	result := BatchResult{Op: p.Op, Total: len(p.Items), Items: make([]BatchItemResult, 0, len(p.Items))}
	for index, item := range p.Items {
		entry := callBatchItem(index, p.Op, item, root, exec)
		if entry.OK {
			result.Succeeded++
		} else {
			result.Failed++
		}
		result.Items = append(result.Items, entry)
	}
	return encode(result)
}

// validateBatchOp 只接受三种已定义的批量动作。
func validateBatchOp(op string) *pluginapi.Error {
	switch op {
	case "delete", "move", "copy":
		return nil
	}
	return invalid("op 必须是 delete / move / copy 之一")
}

// callBatchItem 把单条条目转成对应方法的入参形状并复用同一套处理函数，
// 保证批量与单条走的路径校验、覆盖策略、回收站语义完全一致。
func callBatchItem(index int, op string, item batchItem, root string, exec Executor) BatchItemResult {
	out := BatchItemResult{Index: index, Op: op, Path: item.Path, From: item.Src, To: item.Dst}
	var (
		data json.RawMessage
		perr *pluginapi.Error
	)
	switch op {
	case "delete":
		params, err := json.Marshal(deleteParams{Path: item.Path, Recursive: item.Recursive, Permanent: item.Permanent})
		if err != nil {
			out.Error = &BatchError{Code: pluginapi.E_INTERNAL, Message: err.Error()}
			return out
		}
		data, perr = callDelete(params, root, exec)
	case "move":
		params, err := json.Marshal(moveParams{Src: item.Src, Dst: item.Dst, Overwrite: item.Overwrite})
		if err != nil {
			out.Error = &BatchError{Code: pluginapi.E_INTERNAL, Message: err.Error()}
			return out
		}
		data, perr = callMove(params, root, exec)
	case "copy":
		params, err := json.Marshal(copyParams{Src: item.Src, Dst: item.Dst, Overwrite: item.Overwrite})
		if err != nil {
			out.Error = &BatchError{Code: pluginapi.E_INTERNAL, Message: err.Error()}
			return out
		}
		data, perr = callCopy(params, root, exec)
	default:
		out.Error = &BatchError{Code: pluginapi.E_INVALID, Message: "不支持的批量动作"}
		return out
	}
	if perr != nil {
		out.Error = &BatchError{Code: perr.Code, Message: perr.Message}
		return out
	}
	out.OK = true
	if op == "delete" {
		var deleted DeleteResult
		if err := json.Unmarshal(data, &deleted); err == nil {
			out.TrashID = deleted.TrashID
		}
	}
	return out
}
