package fileop

import (
	"errors"
	"io/fs"
	"os"
)

// 本文件是「目标不存在才改名」的**降级实现**，只在平台没有原子原语时使用：
//   - 非 Linux（macOS / Windows）：没有 renameat2 等价的一次性原语；
//   - 非 Tier A 的 Linux 架构（G-16 只承诺 linux/amd64 与 linux/arm64 的原子路径）。
//
// 已知窗口（必须显式记录，禁止当成原子实现宣传）：
// 先 Lstat 判断目标不存在，再调用 os.Rename；两条路径之间目标被**其它进程/实例**
// 创建时，os.Rename 会静默覆盖它。因此本降级路径只保证「同进程内由本插件串行化」的
// 场景，不保证跨进程互斥——真实的多实例竞争由 Linux 的 renameat2(RENAME_NOREPLACE)
// 收口（见 rename_noreplace_linux.go）。
func renameNoReplaceChecked(src, dst string) error {
	if _, err := os.Lstat(dst); err == nil {
		return fs.ErrExist
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return os.Rename(src, dst)
}
