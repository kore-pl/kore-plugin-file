// Package e2e 用真实插件二进制 + Unix socket 跑 L2 全链路，作为打包前的最终验收。
package e2e

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// buildPlugin 用当前源码构建插件二进制，保证 E2E 测的是工作树而不是旧产物。
func buildPlugin(t *testing.T) string {
	t.Helper()
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if preset := os.Getenv("KORE_FILE_E2E_BIN"); preset != "" {
		if !filepath.IsAbs(preset) {
			preset = filepath.Join(repoRoot, preset)
		}
		if _, err := os.Stat(preset); err != nil {
			t.Fatalf("KORE_FILE_E2E_BIN 指向的二进制不可用: %v", err)
		}
		t.Logf("使用预构建插件二进制: %s", preset)
		return preset
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("缺少 go 工具链，跳过 E2E")
	}
	bin := filepath.Join(t.TempDir(), "file")
	build := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-o", bin, ".")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("构建插件失败: %v\n%s", err, out)
	}
	return bin
}

// startPlugin 起 UDS 监听并拉起插件进程，返回握手后的会话。
func startPlugin(t *testing.T, bin, root string) *pluginSession {
	t.Helper()
	sockDir, err := os.MkdirTemp("", "kore-e2e-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	sock := filepath.Join(sockDir, "plugin.sock")
	listener, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("监听 UDS 失败: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	unixListener, ok := listener.(*net.UnixListener)
	if ok {
		_ = unixListener.SetDeadline(time.Now().Add(15 * time.Second))
	}

	cmd := exec.Command(bin)
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	cmd.Env = append(os.Environ(),
		"KORE_PLUGIN_SOCK="+sock,
		"KORE_FILE_ROOT="+root,
		"KORE_PLUGIN_ID=file",
		"KORE_LOG_LEVEL=error",
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动插件进程失败: %v", err)
	}
	session := &pluginSession{t: t, cmd: cmd, done: make(chan error, 1), stderr: stderr}
	t.Cleanup(func() {
		select {
		case <-session.done:
		default:
			_ = cmd.Process.Kill()
			<-session.done
		}
		if t.Failed() {
			t.Logf("插件 stderr:\n%s", stderr.String())
		}
	})

	conn, err := listener.Accept()
	if err != nil {
		t.Fatalf("等待插件连接失败: %v", err)
	}
	if ok {
		_ = unixListener.SetDeadline(time.Time{})
	}
	session.conn = conn
	t.Cleanup(func() { _ = conn.Close() })
	go func() {
		session.done <- cmd.Wait()
		close(session.done)
	}()

	request := session.readRequest()
	if request.Method != "kore.hello" {
		t.Fatalf("首帧方法=%q，期望 kore.hello", request.Method)
	}
	reply := &pluginapi.Response{
		Type: pluginapi.TypeResponse, ID: request.ID, ReqID: request.ReqID,
		Result: json.RawMessage(`{"ok":true,"protocol":"kore.plugin.v1"}`),
	}
	if err := pluginapi.WriteJSON(conn, reply); err != nil {
		t.Fatalf("回写 hello 失败: %v", err)
	}
	return session
}
