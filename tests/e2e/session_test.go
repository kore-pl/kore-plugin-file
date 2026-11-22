package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"testing"
	"time"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// pluginSession 是假内核侧的一条已握手连接。
type pluginSession struct {
	t      *testing.T
	conn   net.Conn
	seq    int
	cmd    *exec.Cmd
	done   chan error
	stderr *bytes.Buffer
}

// readRequest 读一帧并断言是 request。
func (s *pluginSession) readRequest() *pluginapi.Request {
	s.t.Helper()
	_ = s.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	payload, err := pluginapi.ReadFrame(s.conn, pluginapi.MaxFrameSize)
	if err != nil {
		s.t.Fatalf("读帧失败: %v", err)
	}
	env, err := pluginapi.DecodeFrame(payload)
	if err != nil {
		s.t.Fatalf("解码帧失败: %v", err)
	}
	request, ok := env.(*pluginapi.Request)
	if !ok {
		s.t.Fatalf("帧类型是 %T，不是 request", env)
	}
	return request
}

// call 发一个方法调用并返回结果或协议错误；步骤与 JSON 全部落日志。
func (s *pluginSession) call(method, params string) (json.RawMessage, *pluginapi.Error) {
	s.t.Helper()
	s.seq++
	request := &pluginapi.Request{
		Type:   pluginapi.TypeRequest,
		ID:     fmt.Sprintf("id-%d", s.seq),
		ReqID:  fmt.Sprintf("r-%d", s.seq),
		Method: method,
		Params: json.RawMessage(params),
	}
	if err := pluginapi.WriteJSON(s.conn, request); err != nil {
		s.t.Fatalf("%s 写请求失败: %v", method, err)
	}
	_ = s.conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	payload, err := pluginapi.ReadFrame(s.conn, pluginapi.MaxFrameSize)
	if err != nil {
		s.t.Fatalf("%s 读响应失败: %v", method, err)
	}
	env, err := pluginapi.DecodeFrame(payload)
	if err != nil {
		s.t.Fatalf("%s 解码响应失败: %v", method, err)
	}
	response, ok := env.(*pluginapi.Response)
	if !ok {
		s.t.Fatalf("%s 响应帧类型是 %T", method, env)
	}
	return response.Result, response.Error
}

// must 调用并要求成功，返回原始结果 JSON 并打印到测试日志。
func (s *pluginSession) must(method, params string) json.RawMessage {
	s.t.Helper()
	result, perr := s.call(method, params)
	if perr != nil {
		s.t.Fatalf("%s 失败: %s (%s) params=%s", method, perr.Code, perr.Message, params)
	}
	s.t.Logf("STEP %-13s params=%s result=%s", method, params, result)
	return result
}

// shutdown 发 kore.shutdown 并等待进程退出（证明 lazy 插件能真正退出）。
func (s *pluginSession) shutdown() {
	s.t.Helper()
	if _, perr := s.call("kore.shutdown", `{}`); perr != nil {
		s.t.Fatalf("shutdown 失败: %+v", perr)
	}
	select {
	case err := <-s.done:
		if err != nil {
			s.t.Fatalf("插件退出码非 0: %v", err)
		}
	case <-time.After(10 * time.Second):
		s.t.Fatal("插件未在 10s 内退出")
	}
}
