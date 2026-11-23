package fileop

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

// chunkParams 拼 uploadChunk 参数；totalSize < 0 表示不带 total_size。
func chunkParams(path string, offset int64, data []byte, totalSize int64) string {
	base := `{"path":"` + path + `","offset":` + itoa(offset) + `,"data_base64":"` + base64.StdEncoding.EncodeToString(data) + `"`
	if totalSize >= 0 {
		base += `,"total_size":` + itoa(totalSize)
	}
	return base + "}"
}

// itoa 是测试内最小的 int64→string，避免为几处格式化引入额外 import 命名冲突。
func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// TestUploadChunkResumeRoundTrip 断点续传主链路：分块写入 → 查状态 → 续传 → 内容逐字节一致。
func TestUploadChunkResumeRoundTrip(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	payload := []byte("0123456789abcdefghij-断点续传-END")
	first, second := payload[:10], payload[10:]

	var r1 UploadChunkResult
	if perr := mustCall(t, "file.uploadChunk", chunkParams("docs/big.bin", 0, first, int64(len(payload))), root, &r1); perr != nil {
		t.Fatalf("首块失败: %v", perr)
	}
	if r1.Received != 10 || r1.Complete || r1.Replayed {
		t.Fatalf("首块结果异常: %+v", r1)
	}

	var st UploadStatusResult
	if perr := mustCall(t, "file.uploadStatus", `{"path":"docs/big.bin"}`, root, &st); perr != nil {
		t.Fatalf("查状态失败: %v", perr)
	}
	if !st.Exists || st.Received != 10 {
		t.Fatalf("状态异常: %+v", st)
	}

	var r2 UploadChunkResult
	if perr := mustCall(t, "file.uploadChunk", chunkParams("docs/big.bin", st.Received, second, int64(len(payload))), root, &r2); perr != nil {
		t.Fatalf("续传失败: %v", perr)
	}
	if !r2.Complete || r2.Received != int64(len(payload)) {
		t.Fatalf("续传结果异常: %+v", r2)
	}
	onDisk, err := os.ReadFile(filepath.Join(root, "docs", "big.bin"))
	if err != nil || string(onDisk) != string(payload) {
		t.Fatalf("磁盘内容不一致: %q err=%v", onDisk, err)
	}
}

// TestUploadChunkDuplicateOffsetReplays 重复上传已收分块（丢 ACK 重试）必须幂等，不重复写入。
func TestUploadChunkDuplicateOffsetReplays(t *testing.T) {
	root := t.TempDir()
	payload := []byte("AAAAABBBBB")
	if perr := mustCall(t, "file.uploadChunk", chunkParams("f.bin", 0, payload[:5], 10), root, nil); perr != nil {
		t.Fatal(perr)
	}
	if perr := mustCall(t, "file.uploadChunk", chunkParams("f.bin", 5, payload[5:], 10), root, nil); perr != nil {
		t.Fatal(perr)
	}
	var replay UploadChunkResult
	if perr := mustCall(t, "file.uploadChunk", chunkParams("f.bin", 0, payload[:5], 10), root, &replay); perr != nil {
		t.Fatalf("重传已收分块应幂等，实得错误: %v", perr)
	}
	if !replay.Replayed || replay.Received != 10 {
		t.Fatalf("重传结果异常: %+v", replay)
	}
	onDisk, _ := os.ReadFile(filepath.Join(root, "f.bin"))
	if string(onDisk) != string(payload) {
		t.Fatalf("重传写坏文件: %q", onDisk)
	}
}

// TestUploadChunkOverlapCrossingAppendsTail 跨越已收端点的重传只补未收部分（网络半包恢复）。
func TestUploadChunkOverlapCrossingAppendsTail(t *testing.T) {
	root := t.TempDir()
	payload := []byte("hello-world-tail")
	if perr := mustCall(t, "file.uploadChunk", chunkParams("f.bin", 0, payload[:8], -1), root, nil); perr != nil {
		t.Fatal(perr)
	}
	var r UploadChunkResult
	if perr := mustCall(t, "file.uploadChunk", chunkParams("f.bin", 3, payload[3:], -1), root, &r); perr != nil {
		t.Fatalf("重叠续传失败: %v", perr)
	}
	if r.Received != int64(len(payload)) || r.Replayed {
		t.Fatalf("重叠续传结果异常: %+v", r)
	}
	onDisk, _ := os.ReadFile(filepath.Join(root, "f.bin"))
	if string(onDisk) != string(payload) {
		t.Fatalf("内容不一致: %q", onDisk)
	}
}

// TestUploadChunkConflictAndGap 重叠区不一致与跳空 offset 都必须 E_CONFLICT，不静默覆盖/填充。
func TestUploadChunkConflictAndGap(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "f.bin"), []byte("original-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if perr := mustCall(t, "file.uploadChunk", chunkParams("f.bin", 0, []byte("XXXX"), -1), root, nil); perr == nil || perr.Code != "E_CONFLICT" {
		t.Fatalf("重叠不一致期望 E_CONFLICT，实得 %+v", perr)
	}
	if perr := mustCall(t, "file.uploadChunk", chunkParams("f.bin", 100, []byte("gap"), -1), root, nil); perr == nil || perr.Code != "E_CONFLICT" {
		t.Fatalf("跳空 offset 期望 E_CONFLICT，实得 %+v", perr)
	}
	onDisk, _ := os.ReadFile(filepath.Join(root, "f.bin"))
	if string(onDisk) != "original-data" {
		t.Fatalf("冲突后文件被改动: %q", onDisk)
	}
}

// TestUploadChunkRejectsUnsafeAndOversize 路径穿越 / 目录目标 / total_size 越界都必须 E_INVALID。
func TestUploadChunkRejectsUnsafeAndOversize(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ name, params string }{
		{"路径穿越", chunkParams("../escape.bin", 0, []byte("x"), -1)},
		{"根外绝对路径", chunkParams("/etc/passwd", 0, []byte("x"), -1)},
		{"目标是目录", chunkParams("dir", 0, []byte("x"), -1)},
		{"越过 total_size", chunkParams("a.bin", 4, []byte("12345"), 8)},
		{"空分块", chunkParams("a.bin", 0, nil, -1)},
		{"负 offset", chunkParams("a.bin", -1, []byte("x"), -1)},
	}
	for _, tc := range cases {
		if perr := mustCall(t, "file.uploadChunk", tc.params, root, nil); perr == nil || perr.Code != "E_INVALID" {
			t.Fatalf("%s 期望 E_INVALID，实得 %+v", tc.name, perr)
		}
	}
}

// TestUploadStatusMissing 目标不存在时返回 received=0 / exists=false（客户端从 0 开始）。
func TestUploadStatusMissing(t *testing.T) {
	root := t.TempDir()
	var st UploadStatusResult
	if perr := mustCall(t, "file.uploadStatus", `{"path":"nope.bin"}`, root, &st); perr != nil {
		t.Fatalf("查状态失败: %v", perr)
	}
	if st.Exists || st.Received != 0 {
		t.Fatalf("不存在时状态异常: %+v", st)
	}
	if perr := mustCall(t, "file.uploadStatus", `{"path":"../x"}`, root, nil); perr == nil || perr.Code != "E_INVALID" {
		t.Fatalf("status 路径穿越期望 E_INVALID，实得 %+v", perr)
	}
}
