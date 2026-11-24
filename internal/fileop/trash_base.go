package fileop

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// trashFallback 返回整机视图（root="/"）下的回收站根目录。
//
// 为什么需要兜底：缺省根是 "/"（见 root.go），低权插件进程无法在 / 下创建
// /.trash；不兜底时 file.delete 会在回收站创建阶段失败。优先用内核注入的
// KORE_PLUGIN_DATA（0700、归属插件用户），其次用户缓存目录，最后系统临时目录。
// KORE_FILE_TRASH 可显式覆盖（绝对路径）。
func trashFallback() string {
	if raw := strings.TrimSpace(os.Getenv("KORE_FILE_TRASH")); raw != "" && filepath.IsAbs(raw) {
		return filepath.Clean(raw)
	}
	if data := strings.TrimSpace(os.Getenv("KORE_PLUGIN_DATA")); data != "" && filepath.IsAbs(data) {
		return filepath.Join(data, "trash")
	}
	if cache, err := os.UserCacheDir(); err == nil && cache != "" {
		return filepath.Join(cache, "kore-file-trash")
	}
	return filepath.Join(os.TempDir(), "kore-file-trash")
}

// trashBase 计算并确保回收站根目录可用（create=true 时按需创建）。
// 约束：配置根不是 "/" 时，回收站必须在根的真实路径之内，且自身必须是真实目录
// （不得是符号链接）——否则删除会把文件搬到根外，等于开了一个新的越界口。
// 配置根为 "/"（整机视图）时改用 trashFallback：否则低权进程在 / 下建不出 /.trash。
func trashBase(root string, create bool) (string, bool, *pluginapi.Error) {
	realRoot, perr := evalRoot(root)
	if perr != nil {
		return "", false, perr
	}
	outsideRoot := realRoot == "/"
	base := filepath.Join(realRoot, TrashDirName)
	if outsideRoot {
		base = trashFallback()
	}
	info, err := os.Lstat(base)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if !create {
			return "", false, nil
		}
		if err := os.MkdirAll(filepath.Join(base, trashEntriesDirName), 0o700); err != nil {
			return "", false, classify(err)
		}
	case err != nil:
		return "", false, classify(err)
	case info.Mode()&fs.ModeSymlink != 0:
		return "", false, invalid("回收站目录 " + TrashDirName + " 是符号链接，拒绝使用")
	case !info.IsDir():
		return "", false, invalid("回收站目录 " + TrashDirName + " 被同名文件占用，请先移走")
	}
	if err := os.Chmod(base, 0o700); err != nil {
		return "", false, classify(err)
	}
	real, err := filepath.EvalSymlinks(base)
	if err != nil {
		return "", false, classify(err)
	}
	if outsideRoot {
		// 兜底目录由插件自己创建（0700）；用解析后的真实路径，避免符号链接父目录
		//（如 macOS 的 /tmp -> /private/tmp）造成同一回收站出现两个路径。
		base = real
	} else if real != base || !within(realRoot, real) {
		return "", false, invalid("回收站路径经符号链接解析后越出配置根目录")
	}
	entries := filepath.Join(base, trashEntriesDirName)
	if create {
		if err := ensureTrashSubdir(entries); err != nil {
			return "", false, classify(err)
		}
	} else if _, err := os.Lstat(entries); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return base, true, nil
		}
		return "", false, classify(err)
	}
	return base, true, nil
}

// ensureTrashSubdir 确保条目目录存在且是真实目录（不跟随符号链接）。
func ensureTrashSubdir(entries string) error {
	info, err := os.Lstat(entries)
	if err == nil {
		if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
			return newOpError(ErrInvalid, "回收站条目目录不可用：必须是配置根内的真实目录")
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	return os.Mkdir(entries, 0o700)
}
