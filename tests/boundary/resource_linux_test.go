//go:build linux

package boundary

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/pengtang1024/kore-plugin-file/internal/fileop"
	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// 资源耗尽用例走**真实注入**，不使用任何进程内故障旗标（tests/fault 的 FaultDisk 是旗标，
// 只验证错误映射，不能证明真实 ENOSPC/EFBIG 路径）。注入手段是内核 RLIMIT_FSIZE：
// 在隔离子进程里把文件大小上限压到 16 KiB，任何超过上限的写盘都会由内核拒绝（EFBIG）。

const (
	// rlimitChildEnv 标记「当前进程是资源耗尽子进程」。
	rlimitChildEnv = "KORE_FILE_BOUNDARY_RLIMIT_CHILD"
	// rlimitFileLimit 是子进程的文件大小硬上限：16 KiB。
	rlimitFileLimit = 16 * 1024
)

// TestResourceExhaustionFileSizeLimit 在隔离子进程中真实注入内核文件大小上限，
// 断言：① 裸写超限内容确实被内核拒绝（EFBIG，证明注入为真，不是测试自说自话）；
// ② 插件 file.write 同样内容返回 E_INTERNAL；③ 失败后目标文件不落地、无临时文件残留。
//
// 用子进程而不是本进程 Setrlimit：RLIMIT_FSIZE 是进程级全局状态，本包其它并发用例
// 也在写文件，必须隔离，避免限制泄漏造成假失败（那才是真正的状态串扰）。
func TestResourceExhaustionFileSizeLimit(t *testing.T) {
	t.Parallel()

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("取测试二进制路径失败: %v", err)
	}
	cmd := exec.Command(exe, "-test.run=^TestResourceExhaustionRLimitChild$", "-test.v")
	cmd.Env = append(os.Environ(), rlimitChildEnv+"=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("资源耗尽子进程失败: %v\n子进程输出:\n%s", err, out)
	}
	text := string(out)
	for _, marker := range []string{"REAL_EFBIG_INJECTION_OK", "PLUGIN_WRITE_REJECTED_OK"} {
		if !strings.Contains(text, marker) {
			t.Fatalf("子进程缺少证据标记 %q，输出:\n%s", marker, text)
		}
	}
	if testing.Verbose() {
		t.Logf("隔离子进程真实注入证据:\n%s", text)
	}
}

// TestResourceExhaustionRLimitChild 是资源耗尽子进程主体；父测试通过环境变量唤起，
// 直接 `go test` 时自动 Skip（避免在开发机上把整个测试进程的文件大小限制压小）。
func TestResourceExhaustionRLimitChild(t *testing.T) {
	if os.Getenv(rlimitChildEnv) != "1" {
		t.Skip("仅作为 TestResourceExhaustionFileSizeLimit 的隔离子进程运行")
	}

	// SIGXFSZ 默认动作是终止进程；忽略后写超限由内核返回 EFBIG，才能断言错误码。
	signal.Ignore(syscall.SIGXFSZ)

	var old syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &old); err != nil {
		t.Fatalf("读取 RLIMIT_FSIZE 失败: %v", err)
	}
	limit := syscall.Rlimit{Cur: rlimitFileLimit, Max: rlimitFileLimit}
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &limit); err != nil {
		t.Fatalf("设置 RLIMIT_FSIZE 失败: %v", err)
	}
	t.Cleanup(func() { _ = syscall.Setrlimit(syscall.RLIMIT_FSIZE, &old) })

	// ① 注入有效性自证：裸写 32 KiB 必须被内核以 EFBIG 拒绝。
	probeDir := t.TempDir()
	probeErr := os.WriteFile(filepath.Join(probeDir, "probe.bin"), bytes.Repeat([]byte("x"), rlimitFileLimit*2), 0o644)
	if !errors.Is(probeErr, syscall.EFBIG) {
		t.Fatalf("RLIMIT_FSIZE 未生效：裸写超限内容的错误=%v（期望 EFBIG）", probeErr)
	}
	t.Log("REAL_EFBIG_INJECTION_OK 裸写 32KiB → EFBIG，内核上限已生效")

	// ② 插件写同样超限内容：必须失败并归到 E_INTERNAL，不能"报成功但没写全"。
	root := t.TempDir()
	content := strings.Repeat("K", rlimitFileLimit*2)
	params, err := json.Marshal(writeParams{Path: "big.txt", Content: content})
	if err != nil {
		t.Fatalf("构造 file.write 参数失败: %v", err)
	}
	result, perr := fileop.Call("file.write", params, root, nil)
	if perr == nil {
		t.Fatalf("RLIMIT_FSIZE 生效时 file.write 仍返回成功（结果=%s）", result)
	}
	if perr.Code != pluginapi.E_INTERNAL {
		t.Fatalf("file.write 错误码=%q，期望 E_INTERNAL（message=%q）", perr.Code, perr.Message)
	}
	if !strings.Contains(perr.Message, "文件系统操作失败") {
		t.Fatalf("file.write 错误信息 %q 未体现文件系统失败", perr.Message)
	}
	t.Logf("插件 file.write 拒绝: %s: %s", perr.Code, perr.Message)
	t.Log("PLUGIN_WRITE_REJECTED_OK 超限写入返回 E_INTERNAL")

	// ③ 失败不留半文件、不留临时文件（原子写失败必须完整回滚）。
	if _, err := os.Stat(filepath.Join(root, "big.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("超限写入失败后目标文件不应存在: stat err=%v", err)
	}
	leftovers, err := filepath.Glob(filepath.Join(root, ".kore-file-*.tmp"))
	if err != nil {
		t.Fatalf("扫描临时文件失败: %v", err)
	}
	if len(leftovers) != 0 {
		t.Fatalf("超限写入失败后残留临时文件: %v", leftovers)
	}
	t.Logf("失败后根目录无 big.txt、无 .kore-file-*.tmp 残留（%s）", fmt.Sprint(root))
}
