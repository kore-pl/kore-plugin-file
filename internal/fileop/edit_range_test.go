package fileop

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// b64 / base64Decode / jsonRaw 是测试侧最小编解码工具，避免用例里重复展开。
func b64(data []byte) string                   { return base64.StdEncoding.EncodeToString(data) }
func base64Decode(text string) ([]byte, error) { return base64.StdEncoding.DecodeString(text) }

// shaOf 返回整文件摘要，供 writeRange 乐观锁与结果断言使用。
func shaOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// TestReadRangeWindow 按字节窗口读取：size/returned/truncated 与内容一致。
func TestReadRangeWindow(t *testing.T) {
	root := t.TempDir()
	payload := make([]byte, 1000)
	for i := range payload {
		payload[i] = byte('a' + i%26)
	}
	if err := os.WriteFile(filepath.Join(root, "f.txt"), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	var mid ReadRangeResult
	if perr := mustCall(t, "file.readRange", `{"path":"f.txt","offset":100,"limit":50}`, root, &mid); perr != nil {
		t.Fatalf("readRange 失败: %v", perr)
	}
	if mid.Size != 1000 || mid.Returned != 50 || !mid.Truncated {
		t.Fatalf("窗口元信息异常: %+v", mid)
	}
	got, err := base64Decode(mid.DataBase64)
	if err != nil || string(got) != string(payload[100:150]) {
		t.Fatalf("窗口内容不一致: %q err=%v", got, err)
	}
	var tail ReadRangeResult
	if perr := mustCall(t, "file.readRange", `{"path":"f.txt","offset":980}`, root, &tail); perr != nil {
		t.Fatalf("尾段读取失败: %v", perr)
	}
	if tail.Returned != 20 || tail.Truncated {
		t.Fatalf("尾段应读完且不截断: %+v", tail)
	}
	// 默认 limit 覆盖小文件：不传 limit 也整读。
	var full ReadRangeResult
	if perr := mustCall(t, "file.readRange", `{"path":"f.txt"}`, root, &full); perr != nil {
		t.Fatalf("默认窗口读取失败: %v", perr)
	}
	if full.Returned != 1000 || full.Truncated {
		t.Fatalf("默认窗口应读完整个小文件: %+v", full)
	}
}

// TestReadRangeRejects 路径穿越 / offset 越界 / limit 超上限 / 目录都按 E_INVALID 拒绝。
func TestReadRangeRejects(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ name, params string }{
		{"路径穿越", `{"path":"../etc/passwd"}`},
		{"绝对路径", `{"path":"/etc/passwd"}`},
		{"offset 越界", `{"path":"f.txt","offset":100}`},
		{"limit 超上限", `{"path":"f.txt","limit":999999999}`},
		{"负 offset", `{"path":"f.txt","offset":-1}`},
		{"目录", `{"path":"dir"}`},
		{"回收站目录", `{"path":".trash/x"}`},
	}
	for _, tc := range cases {
		if perr := mustCall(t, "file.readRange", tc.params, root, nil); perr == nil || perr.Code != "E_INVALID" {
			t.Fatalf("%s 期望 E_INVALID，实得 %+v", tc.name, perr)
		}
	}
	if perr := mustCall(t, "file.readRange", `{"path":"nope.txt"}`, root, nil); perr == nil || perr.Code != "E_NOTFOUND" {
		t.Fatalf("不存在文件期望 E_NOTFOUND，实得 %+v", perr)
	}
}

// TestWriteRangeOverwriteAndExtend 覆盖中间不改变尾部；offset==size 追加扩展。
func TestWriteRangeOverwriteAndExtend(t *testing.T) {
	root := t.TempDir()
	base := []byte("0123456789ABCDEFGHIJ")
	if err := os.WriteFile(filepath.Join(root, "f.bin"), base, 0o644); err != nil {
		t.Fatal(err)
	}
	var r WriteRangeResult
	if perr := mustCall(t, "file.writeRange", `{"path":"f.bin","offset":5,"data_base64":"`+b64([]byte("xyz"))+`"}`, root, &r); perr != nil {
		t.Fatalf("覆盖写失败: %v", perr)
	}
	want := []byte("01234xyz89ABCDEFGHIJ")
	onDisk, _ := os.ReadFile(filepath.Join(root, "f.bin"))
	if string(onDisk) != string(want) {
		t.Fatalf("覆盖写结果异常: %q", onDisk)
	}
	if r.Size != int64(len(want)) || r.Written != 3 || r.SHA256 != shaOf(want) {
		t.Fatalf("覆盖写元信息异常: %+v", r)
	}
	var ext WriteRangeResult
	if perr := mustCall(t, "file.writeRange", `{"path":"f.bin","offset":20,"data_base64":"`+b64([]byte("-tail"))+`"}`, root, &ext); perr != nil {
		t.Fatalf("追加写失败: %v", perr)
	}
	onDisk, _ = os.ReadFile(filepath.Join(root, "f.bin"))
	if string(onDisk) != string(want)+"-tail" || ext.Size != 25 {
		t.Fatalf("追加写结果异常: %q size=%d", onDisk, ext.Size)
	}
}

// TestWriteRangeOptimisticLock expected_sha256 匹配才落盘；陈旧摘要回 E_CONFLICT 且内容不变。
func TestWriteRangeOptimisticLock(t *testing.T) {
	root := t.TempDir()
	base := []byte("hello world")
	if err := os.WriteFile(filepath.Join(root, "f.txt"), base, 0o644); err != nil {
		t.Fatal(err)
	}
	stale := shaOf(base)
	params := `{"path":"f.txt","offset":0,"data_base64":"` + b64([]byte("HELLO")) + `","expected_sha256":"` + stale + `"}`
	if perr := mustCall(t, "file.writeRange", params, root, nil); perr != nil {
		t.Fatalf("正确摘要应允许写入: %v", perr)
	}
	if perr := mustCall(t, "file.writeRange", params, root, nil); perr == nil || perr.Code != "E_CONFLICT" {
		t.Fatalf("陈旧摘要期望 E_CONFLICT，实得 %+v", perr)
	}
	onDisk, _ := os.ReadFile(filepath.Join(root, "f.txt"))
	if string(onDisk) != "HELLO world" {
		t.Fatalf("冲突写入改动了文件: %q", onDisk)
	}
}

// TestWriteRangeGapAndUnsafe 跳空写回 E_CONFLICT；穿越 / 目录 / 不存在按协议错误拒绝。
func TestWriteRangeGapAndUnsafe(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `"data_base64":"` + b64([]byte("x")) + `"`
	if perr := mustCall(t, "file.writeRange", `{"path":"f.txt","offset":10,`+body+`}`, root, nil); perr == nil || perr.Code != "E_CONFLICT" {
		t.Fatalf("跳空写期望 E_CONFLICT，实得 %+v", perr)
	}
	if perr := mustCall(t, "file.writeRange", `{"path":"../x","offset":0,`+body+`}`, root, nil); perr == nil || perr.Code != "E_INVALID" {
		t.Fatalf("路径穿越期望 E_INVALID，实得 %+v", perr)
	}
	if perr := mustCall(t, "file.writeRange", `{"path":"dir","offset":0,`+body+`}`, root, nil); perr == nil || perr.Code != "E_INVALID" {
		t.Fatalf("目录期望 E_INVALID，实得 %+v", perr)
	}
	if perr := mustCall(t, "file.writeRange", `{"path":"nope.txt","offset":0,`+body+`}`, root, nil); perr == nil || perr.Code != "E_NOTFOUND" {
		t.Fatalf("不存在期望 E_NOTFOUND，实得 %+v", perr)
	}
	onDisk, _ := os.ReadFile(filepath.Join(root, "f.txt"))
	if strings.Contains(string(onDisk), "x") || len(onDisk) != 3 {
		t.Fatalf("失败写入改动了文件: %q", onDisk)
	}
}

// TestWriteRangeConcurrentOneWins 两个会话基于同一摘要并发保存：恰好一个成功，另一个 E_CONFLICT（不丢更新）。
func TestWriteRangeConcurrentOneWins(t *testing.T) {
	root := t.TempDir()
	base := []byte("base content")
	if err := os.WriteFile(filepath.Join(root, "f.txt"), base, 0o644); err != nil {
		t.Fatal(err)
	}
	expected := shaOf(base)
	var wg sync.WaitGroup
	errs := make([]*pluginapi.Error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := `{"path":"f.txt","offset":0,"data_base64":"` + b64([]byte{byte('A' + i)}) + `","expected_sha256":"` + expected + `"}`
			_, perr := Call("file.writeRange", jsonRaw(body), root, nil)
			errs[i] = perr
		}(i)
	}
	wg.Wait()
	okCount, conflictCount := 0, 0
	for _, perr := range errs {
		switch {
		case perr == nil:
			okCount++
		case perr.Code == "E_CONFLICT":
			conflictCount++
		}
	}
	if okCount != 1 || conflictCount != 1 {
		t.Fatalf("并发保存应恰好一成一冲突，实得 ok=%d conflict=%d (%+v)", okCount, conflictCount, errs)
	}
}
