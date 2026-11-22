package fault

import (
	"testing"

	"github.com/pengtang1024/kore-plugin-file/internal/fileop"
)

// TestCgroupLimitFault 注入 cgroup 超限，期望三方法返回 E_UNSUPPORTED。
func TestCgroupLimitFault(t *testing.T) {
	assertFault(t, fileop.FaultCgroup, "E_UNSUPPORTED", "cgroup")
}
