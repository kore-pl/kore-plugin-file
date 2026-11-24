package fileop

import (
	"errors"
	"io/fs"
	"strings"
	"syscall"
	"testing"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// TestMapFSErrorCarriesErrnoAndHint 文件系统错误必须带 errno 上下文与「怎么办」，
// 且不泄露宿主绝对路径（HANDOFF B.6 #8：errno 诊断信息缺）。
func TestMapFSErrorCarriesErrnoAndHint(t *testing.T) {
	const hostPath = "/home/pt/private/secret.txt"
	cases := []struct {
		name     string
		err      error
		wantCode string
		wantText []string
	}{
		{
			name:     "EACCES 权限不足给操作建议",
			err:      &fs.PathError{Op: "open", Path: hostPath, Err: syscall.EACCES},
			wantCode: pluginapi.E_INTERNAL,
			wantText: []string{"权限不足", "permission denied", "请检查", "root"},
		},
		{
			name:     "EFBIG 文件系统失败带 errno 与建议",
			err:      &fs.PathError{Op: "write", Path: hostPath, Err: syscall.EFBIG},
			wantCode: pluginapi.E_INTERNAL,
			wantText: []string{"文件系统操作失败", "write", "file too large", "请检查"},
		},
		{
			name:     "ENOSPC 文件系统失败带 errno 与建议",
			err:      &fs.PathError{Op: "write", Path: hostPath, Err: syscall.ENOSPC},
			wantCode: pluginapi.E_INTERNAL,
			wantText: []string{"文件系统操作失败", "no space left on device", "磁盘剩余空间"},
		},
		{
			name:     "非 PathError 也带错误上下文",
			err:      errors.New("backend refused"),
			wantCode: pluginapi.E_INTERNAL,
			wantText: []string{"文件系统操作失败", "backend refused", "请检查"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mapFSError(tc.err)
			if got == nil || got.Code != tc.wantCode {
				t.Fatalf("mapFSError(%v)=%+v，期望 code=%s", tc.err, got, tc.wantCode)
			}
			for _, want := range tc.wantText {
				if !strings.Contains(got.Message, want) {
					t.Errorf("错误信息 %q 未含关键片段 %q", got.Message, want)
				}
			}
			if strings.Contains(got.Message, hostPath) {
				t.Errorf("错误信息泄露宿主绝对路径 %q: %q", hostPath, got.Message)
			}
		})
	}
}
