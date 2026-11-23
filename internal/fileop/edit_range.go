package fileop

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"sync"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

const (
	// DefaultReadRangeLimit 是 readRange 未显式给 limit 时的默认窗口（256 KiB）。
	DefaultReadRangeLimit int64 = 256 << 10
	// MaxReadRangeLimit 是单次 readRange 的上限，取单帧可承载量（≈5.95 MiB）。
	MaxReadRangeLimit int64 = MaxInlineTransferBytes
)

// editLocks 是进程内按绝对路径的串行锁：同一文件的读-改-写不交叉。
// 内核单插件单进程，进程内串行 + expected_sha256 乐观锁 = 不丢更新；不引入 flock：
// 根目录由本插件独占，跨进程没有并发写同一棵树的合法场景。
var editLocks = struct {
	sync.Mutex
	m map[string]*sync.Mutex
}{m: make(map[string]*sync.Mutex)}

// lockPath 取出并锁住 path 的互斥量，返回解锁函数。
func lockPath(abs string) func() {
	editLocks.Lock()
	mu := editLocks.m[abs]
	if mu == nil {
		mu = &sync.Mutex{}
		editLocks.m[abs] = mu
	}
	editLocks.Unlock()
	mu.Lock()
	return mu.Unlock
}

// readRangeParams 是 file.readRange 的显式入参形状。
type readRangeParams struct {
	Path   string `json:"path"`
	Offset int64  `json:"offset,omitempty"`
	Limit  int64  `json:"limit,omitempty"`
}

// ReadRangeResult 是 file.readRange 的结果；data_base64 便于二进制编辑器直接解码。
type ReadRangeResult struct {
	Path       string `json:"path"`
	Offset     int64  `json:"offset"`
	Size       int64  `json:"size"`
	Returned   int64  `json:"returned"`
	Truncated  bool   `json:"truncated"`
	DataBase64 string `json:"data_base64"`
}

// writeRangeParams 是 file.writeRange 的显式入参形状；expected_sha256 是整文件写前摘要（乐观锁）。
type writeRangeParams struct {
	Path           string `json:"path"`
	Offset         int64  `json:"offset"`
	DataBase64     string `json:"data_base64"`
	ExpectedSHA256 string `json:"expected_sha256,omitempty"`
}

// WriteRangeResult 是 file.writeRange 的结果；sha256 是写后整文件摘要，供下一次乐观锁使用。
type WriteRangeResult struct {
	Path    string `json:"path"`
	Offset  int64  `json:"offset"`
	Written int64  `json:"written"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
}

// callReadRange 处理 file.readRange：按字节窗口读取，大文件不整读。
func callReadRange(raw json.RawMessage, root string) (json.RawMessage, *pluginapi.Error) {
	var p readRangeParams
	if perr := decodeParams(raw, &p); perr != nil {
		return nil, perr
	}
	if p.Path == "" {
		return nil, invalid("path 不能为空")
	}
	if p.Offset < 0 {
		return nil, invalid("offset 不能为负数")
	}
	limit := p.Limit
	if limit == 0 {
		limit = DefaultReadRangeLimit
	}
	if limit < 0 {
		return nil, invalid("limit 不能为负数")
	}
	if limit > MaxReadRangeLimit {
		return nil, invalid("limit 超过单帧上限，请分段读取")
	}
	abs, perr := ResolveExisting(root, p.Path)
	if perr != nil {
		return nil, perr
	}
	unlock := lockPath(abs)
	defer unlock()
	file, err := os.Open(abs)
	if err != nil {
		return nil, classify(err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, classify(err)
	}
	if info.IsDir() {
		return nil, invalid("目标路径是目录，不能按范围读取")
	}
	if p.Offset > info.Size() {
		return nil, invalid("offset 超过文件大小")
	}
	window := limit
	if remain := info.Size() - p.Offset; window > remain {
		window = remain
	}
	buf := make([]byte, window)
	if window > 0 {
		if _, err := io.ReadFull(io.NewSectionReader(file, p.Offset, window), buf); err != nil {
			return nil, classify(err)
		}
	}
	return encode(ReadRangeResult{
		Path:       displayPath(p.Path),
		Offset:     p.Offset,
		Size:       info.Size(),
		Returned:   window,
		Truncated:  p.Offset+window < info.Size(),
		DataBase64: base64.StdEncoding.EncodeToString(buf),
	})
}

// callWriteRange 处理 file.writeRange：在 offset 处覆盖写（不截断），可按整文件 sha256 做乐观锁。
//
// 并发语义（测试锁死）：
//   - 同一路径的读/写在本进程内按 path 互斥串行，不会出现半写交叉；
//   - 跨客户端并发用 expected_sha256 乐观锁：写前摘要与当前文件不一致即 E_CONFLICT，不落盘；
//   - offset 超过当前文件大小视为跳空，直接 E_CONFLICT（不自动补零，避免把编辑错误写成静默损坏）。
func callWriteRange(raw json.RawMessage, root string) (json.RawMessage, *pluginapi.Error) {
	var p writeRangeParams
	if perr := decodeParams(raw, &p); perr != nil {
		return nil, perr
	}
	if p.Path == "" {
		return nil, invalid("path 不能为空")
	}
	if p.Offset < 0 {
		return nil, invalid("offset 不能为负数")
	}
	data, perr := decodeBase64Payload(p.DataBase64)
	if perr != nil {
		return nil, perr
	}
	abs, perr := ResolveExisting(root, p.Path)
	if perr != nil {
		return nil, perr
	}
	unlock := lockPath(abs)
	defer unlock()
	if st, err := os.Stat(abs); err == nil && st.IsDir() {
		return nil, invalid("目标路径是目录，不能按范围写入")
	}
	file, err := os.OpenFile(abs, os.O_RDWR, 0)
	if err != nil {
		return nil, classify(err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, classify(err)
	}
	if info.IsDir() {
		return nil, invalid("目标路径是目录，不能按范围写入")
	}
	if p.Offset > info.Size() {
		return nil, conflict("offset 超过当前文件大小（不允许跳空写，避免补零损坏）")
	}
	if p.ExpectedSHA256 != "" {
		current, perr := fileSHA256(file)
		if perr != nil {
			return nil, perr
		}
		if current != p.ExpectedSHA256 {
			return nil, conflict("文件已被其他会话修改（sha256 不匹配），请重新读取后再保存")
		}
	}
	if len(data) > 0 {
		if _, err := file.WriteAt(data, p.Offset); err != nil {
			return nil, classify(err)
		}
	}
	if err := file.Sync(); err != nil {
		return nil, classify(err)
	}
	after, err := file.Stat()
	if err != nil {
		return nil, classify(err)
	}
	sum, shaErr := fileSHA256(file)
	if shaErr != nil {
		return nil, shaErr
	}
	return encode(WriteRangeResult{
		Path:    displayPath(p.Path),
		Offset:  p.Offset,
		Written: int64(len(data)),
		Size:    after.Size(),
		SHA256:  sum,
	})
}

// fileSHA256 计算已打开文件的整体摘要（读游标用 ReadAt，不改变文件偏移语义）。
func fileSHA256(file *os.File) (string, *pluginapi.Error) {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, io.NewSectionReader(file, 0, 1<<62)); err != nil {
		return "", classify(err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
