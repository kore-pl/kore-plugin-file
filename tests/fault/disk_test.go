package fault

import (
	"testing"

	"github.com/pengtang1024/kore-plugin-file/internal/fileop"
)

// TestDiskFullFault 注入磁盘满，期望三方法返回 E_INTERNAL。
func TestDiskFullFault(t *testing.T) {
	assertFault(t, fileop.FaultDisk, "E_INTERNAL", "磁盘满")
}
