package fileop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// TestBatchDeletePartialFailure 批量删除：一个缺失不影响其余，逐项给出结果与错误码。
func TestBatchDeletePartialFailure(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "a.txt", "a")
	mustWrite(t, root, "b.txt", "b")
	params := `{"op":"delete","items":[{"path":"a.txt","permanent":true},{"path":"missing.txt"},{"path":"b.txt","permanent":true}]}`
	var got BatchResult
	if perr := mustCall(t, "file.batch", params, root, &got); perr != nil {
		t.Fatalf("批量删除失败: %v", perr)
	}
	if got.Total != 3 || got.Succeeded != 2 || got.Failed != 1 {
		t.Fatalf("批量统计异常: %+v", got)
	}
	if !got.Items[0].OK || got.Items[1].OK || !got.Items[2].OK {
		t.Fatalf("逐项结果异常: %+v", got.Items)
	}
	if got.Items[1].Error == nil || got.Items[1].Error.Code != pluginapi.E_NOTFOUND || got.Items[1].Error.Message == "" {
		t.Fatalf("失败项错误体异常: %+v", got.Items[1].Error)
	}
	for _, name := range []string{"a.txt", "b.txt"} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("%s 应已被删除: %v", name, err)
		}
	}
}

// TestBatchDeleteToTrash 批量删除默认进回收站：逐项 trash_id 可用于恢复。
func TestBatchDeleteToTrash(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "a.txt", "a")
	mustWrite(t, root, "b.txt", "b")
	var got BatchResult
	if perr := mustCall(t, "file.batch", `{"op":"delete","items":[{"path":"a.txt"},{"path":"b.txt"}]}`, root, &got); perr != nil {
		t.Fatalf("批量删除失败: %v", perr)
	}
	if got.Succeeded != 2 {
		t.Fatalf("批量删除应全部成功: %+v", got)
	}
	for _, item := range got.Items {
		if item.TrashID == "" {
			t.Fatalf("条目缺少 trash_id: %+v", item)
		}
	}
	var listing TrashListResult
	if perr := mustCall(t, "file.trash.list", `{}`, root, &listing); perr != nil || listing.Count != 2 {
		t.Fatalf("回收站应有 2 条: %+v err=%v", listing, perr)
	}
	if perr := mustCall(t, "file.trash.restore", `{"id":"`+got.Items[0].TrashID+`"}`, root, nil); perr != nil {
		t.Fatalf("按批量结果恢复失败: %v", perr)
	}
	if data, err := os.ReadFile(filepath.Join(root, "a.txt")); err != nil || string(data) != "a" {
		t.Fatalf("恢复后内容=%q err=%v", data, err)
	}
}

// TestBatchMoveAndCopy 批量 move / copy：目标冲突只影响该条。
func TestBatchMoveAndCopy(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "a.txt", "a")
	mustWrite(t, root, "b.txt", "b")
	mustWrite(t, root, "occupied.txt", "o")
	var moved BatchResult
	params := `{"op":"move","items":[{"src":"a.txt","dst":"a2.txt"},{"src":"b.txt","dst":"occupied.txt"}]}`
	if perr := mustCall(t, "file.batch", params, root, &moved); perr != nil {
		t.Fatalf("批量移动失败: %v", perr)
	}
	if moved.Succeeded != 1 || moved.Failed != 1 || moved.Items[1].Error.Code != pluginapi.E_CONFLICT {
		t.Fatalf("批量移动结果异常: %+v", moved)
	}
	if _, err := os.Stat(filepath.Join(root, "a2.txt")); err != nil {
		t.Fatalf("移动目标缺失: %v", err)
	}
	var copied BatchResult
	if perr := mustCall(t, "file.batch", `{"op":"copy","items":[{"src":"a2.txt","dst":"a3.txt"},{"src":"missing.txt","dst":"x.txt"}]}`, root, &copied); perr != nil {
		t.Fatalf("批量复制失败: %v", perr)
	}
	if copied.Succeeded != 1 || copied.Failed != 1 || copied.Items[1].Error.Code != pluginapi.E_NOTFOUND {
		t.Fatalf("批量复制结果异常: %+v", copied)
	}
	if data, err := os.ReadFile(filepath.Join(root, "a3.txt")); err != nil || string(data) != "a" {
		t.Fatalf("复制内容=%q err=%v", data, err)
	}
}

// TestBatchStructuralValidation 结构性错误让整次调用失败；条目级错误只进逐项结果。
func TestBatchStructuralValidation(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "a.txt", "a")
	items := make([]string, 0, MaxBatchItems+1)
	for index := 0; index <= MaxBatchItems; index++ {
		items = append(items, `{"path":"a.txt"}`)
	}
	tooMany := `{"op":"delete","items":[` + strings.Join(items, ",") + `]}`
	cases := []struct {
		params string
		code   string
	}{
		{`{"op":"","items":[{"path":"a.txt"}]}`, pluginapi.E_INVALID},
		{`{"op":"chmod","items":[{"path":"a.txt"}]}`, pluginapi.E_INVALID},
		{`{"op":"delete","items":[]}`, pluginapi.E_INVALID},
		{tooMany, pluginapi.E_INVALID},
	}
	for _, c := range cases {
		resp, perr := Call("file.batch", json.RawMessage(c.params), root, nil)
		if perr == nil || perr.Code != c.code || resp != nil {
			t.Fatalf("%s 错误=%v，期望 %s", truncate(c.params), perr, c.code)
		}
	}
	// 条目级安全拒绝不算结构性失败：整体仍成功，逐项标 E_INVALID。
	var got BatchResult
	if perr := mustCall(t, "file.batch", `{"op":"delete","items":[{"path":"../escape"},{"path":".trash/x"}]}`, root, &got); perr != nil {
		t.Fatalf("条目级拒绝不应让整次失败: %v", perr)
	}
	if got.Failed != 2 || got.Items[0].Error.Code != pluginapi.E_INVALID || got.Items[1].Error.Code != pluginapi.E_INVALID {
		t.Fatalf("条目级错误码异常: %+v", got)
	}
}
