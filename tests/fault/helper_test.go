// Package fault 对 file 插件做四类故障注入，验证方法级错误返回。
// 每个用例向 fileop.Call 的执行面注入一种故障，断言方法返回的结构化错误码。
package fault

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/pengtang1024/kore-plugin-file/internal/fileop"
)

// faultExecutor 按注入类别让每个方法都失败，模拟四类运行时故障。
type faultExecutor struct {
	kind fileop.FaultKind
}

func (f faultExecutor) List(string) ([]fileop.Entry, error) {
	return nil, fileop.NewFaultError(f.kind, "list 执行面故障")
}

func (f faultExecutor) Read(string, int64) (fileop.ReadData, error) {
	return fileop.ReadData{}, fileop.NewFaultError(f.kind, "read 执行面故障")
}

func (f faultExecutor) Write(string, string, bool) (fileop.WriteData, error) {
	return fileop.WriteData{}, fileop.NewFaultError(f.kind, "write 执行面故障")
}

func (f faultExecutor) Delete(string, bool) error {
	return fileop.NewFaultError(f.kind, "delete 执行面故障")
}

func (f faultExecutor) Mkdir(string, fs.FileMode) error {
	return fileop.NewFaultError(f.kind, "mkdir 执行面故障")
}

func (f faultExecutor) Stat(string) (fileop.StatData, error) {
	return fileop.StatData{}, fileop.NewFaultError(f.kind, "stat 执行面故障")
}

func (f faultExecutor) Upload(string, []byte, bool) (fileop.UploadData, error) {
	return fileop.UploadData{}, fileop.NewFaultError(f.kind, "upload 执行面故障")
}

func (f faultExecutor) Download(string, int64) (fileop.DownloadData, error) {
	return fileop.DownloadData{}, fileop.NewFaultError(f.kind, "download 执行面故障")
}

func (f faultExecutor) Extract(fileop.ExtractRequest) (fileop.ExtractData, error) {
	return fileop.ExtractData{}, fileop.NewFaultError(f.kind, "extract 执行面故障")
}

func (f faultExecutor) Compress([]fileop.CompressSource, string, string) (fileop.CompressData, error) {
	return fileop.CompressData{}, fileop.NewFaultError(f.kind, "compress 执行面故障")
}

func (f faultExecutor) Chmod(string, fs.FileMode, bool) (fileop.ChmodData, error) {
	return fileop.ChmodData{}, fileop.NewFaultError(f.kind, "chmod 执行面故障")
}

func (f faultExecutor) Move(string, string, bool) (fileop.MoveData, error) {
	return fileop.MoveData{}, fileop.NewFaultError(f.kind, "move 执行面故障")
}

func (f faultExecutor) Search(fileop.SearchRequest) (fileop.SearchData, error) {
	return fileop.SearchData{}, fileop.NewFaultError(f.kind, "search 执行面故障")
}

func (f faultExecutor) Copy(string, string, bool) (fileop.CopyData, error) {
	return fileop.CopyData{}, fileop.NewFaultError(f.kind, "copy 执行面故障")
}

func (f faultExecutor) TrashPut(string, string, string, bool) (fileop.TrashEntry, error) {
	return fileop.TrashEntry{}, fileop.NewFaultError(f.kind, "trash.put 执行面故障")
}

func (f faultExecutor) TrashGet(string, string) (fileop.TrashEntry, error) {
	return fileop.TrashEntry{}, fileop.NewFaultError(f.kind, "trash.get 执行面故障")
}

func (f faultExecutor) TrashList(string, int) (fileop.TrashListing, error) {
	return fileop.TrashListing{}, fileop.NewFaultError(f.kind, "trash.list 执行面故障")
}

func (f faultExecutor) TrashRestore(string, string, string, bool) (fileop.TrashEntry, error) {
	return fileop.TrashEntry{}, fileop.NewFaultError(f.kind, "trash.restore 执行面故障")
}

func (f faultExecutor) TrashPurge(string, string, bool) (int, error) {
	return 0, fileop.NewFaultError(f.kind, "trash.purge 执行面故障")
}

// methods 是故障注入覆盖的方法及其合法参数。
// 铁律：每条用例只能依赖 prepareRoot 的种子，不能依赖前面步骤的产物——
// 注入故障后前面的方法必然失败，否则后面的方法会在路径解析阶段就返回 E_NOTFOUND，
// 根本走不到执行面，故障断言会变成假通过。
// file.batch / file.trash.restore / file.trash.purge 的部分失败语义在 batch_test.go 单独验证。
var methods = []struct {
	name   string
	method string
	params string
}{
	{"list", "file.list", `{"path":"docs"}`},
	{"read", "file.read", `{"path":"docs/a.txt"}`},
	{"write", "file.write", `{"path":"docs/a.txt","content":"hello"}`},
	{"search", "file.search", `{"path":"docs","pattern":"*.txt"}`},
	{"copy", "file.copy", `{"src":"docs/a.txt","dst":"docs/copy.txt"}`},
	{"rename", "file.rename", `{"path":"docs/rename-src.txt","name":"rename-dst.txt"}`},
	{"upload", "file.upload", `{"path":"docs","filename":"up.txt","data_base64":"aGVsbG8="}`},
	{"download", "file.download", `{"path":"docs/a.txt"}`},
	{"compress", "file.compress", `{"paths":["docs"],"archive_path":"out.zip"}`},
	{"chmod", "file.chmod", `{"path":"docs/a.txt","mode":"0600"}`},
	{"move", "file.move", `{"src":"docs/a.txt","dst":"docs/b.txt"}`},
	{"trash.list", "file.trash.list", `{}`},
	{"delete", "file.delete", `{"path":"docs/trash-target.txt"}`},
}

// prepareRoot 造一个真实临时根：路径解析必须通过，故障只能来自注入的执行面。
func prepareRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("造 docs 目录失败: %v", err)
	}
	// 回收站目录先建好：trash.list 在回收站不存在时走空列表短路，不会触达执行面。
	if err := os.MkdirAll(filepath.Join(root, fileop.TrashDirName, "entries"), 0o700); err != nil {
		t.Fatalf("造回收站目录失败: %v", err)
	}
	// 每条用例用自己的种子文件，互不依赖：注入故障时前面的步骤不会造出后面依赖的对象。
	for _, name := range []string{"a.txt", "rename-src.txt", "trash-target.txt", "batch1.txt", "batch2.txt"} {
		if err := os.WriteFile(filepath.Join(root, "docs", name), []byte("seed"), 0o644); err != nil {
			t.Fatalf("造种子文件失败: %v", err)
		}
	}
	return root
}
