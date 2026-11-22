package fileop

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// chmodParams 是 file.chmod 的显式入参形状。
type chmodParams struct {
	Path      string `json:"path"`
	Mode      string `json:"mode"`
	Recursive bool   `json:"recursive"`
}

// ChmodResult 是 file.chmod 的结果；changed 是实际被修改的条目数（非递归最多 1）。
type ChmodResult struct {
	Path    string `json:"path"`
	Mode    string `json:"mode"`
	Changed int    `json:"changed"`
}

// ChmodData 是 file.chmod 执行面返回的结果。
type ChmodData struct {
	Changed int
}

// chmodItem 是递归改权限时先收集、校验通过后再统一应用的条目。
type chmodItem struct {
	path string
	mode fs.FileMode
}

// callChmod 处理 file.chmod：mode 是八进制字符串；目录必须保留执行位。
func callChmod(raw json.RawMessage, root string, exec Executor) (json.RawMessage, *pluginapi.Error) {
	var p chmodParams
	if perr := decodeParams(raw, &p); perr != nil {
		return nil, perr
	}
	if p.Path == "" {
		return nil, invalid("path 不能为空")
	}
	display := displayPath(p.Path)
	if display == "." {
		return nil, invalid("不允许修改配置根目录自身的权限")
	}
	mode, perr := parseChmodMode(p.Mode)
	if perr != nil {
		return nil, perr
	}
	// 与 chmod(1) 一致：命令参数上的符号链接解析到目标，再校验目标仍在根内。
	abs, perr := ResolveExisting(root, p.Path)
	if perr != nil {
		return nil, perr
	}
	data, err := exec.Chmod(abs, fs.FileMode(mode), p.Recursive)
	if err != nil {
		return nil, classify(err)
	}
	return encode(ChmodResult{Path: display, Mode: modeString(mode), Changed: data.Changed})
}

// parseChmodMode 只接受 0–0777 的八进制字符串，避免把 "rwx" 或十进制当权限。
func parseChmodMode(raw string) (uint32, *pluginapi.Error) {
	if raw == "" {
		return 0, invalid("mode 不能为空，请传八进制字符串，例如 \"0755\"")
	}
	s := strings.TrimPrefix(strings.TrimPrefix(raw, "0o"), "0O")
	value, err := strconv.ParseUint(s, 8, 32)
	if err != nil || value > 0o777 {
		return 0, invalid("mode 必须是 0 到 0777 的八进制权限，例如 \"0755\"")
	}
	return uint32(value), nil
}

// Chmod 改权限：非递归直接应用；递归先遍历收集并校验，全部合法后再逐项 chmod。
func (realFS) Chmod(abs string, mode fs.FileMode, recursive bool) (ChmodData, error) {
	info, err := os.Lstat(abs)
	if err != nil {
		return ChmodData{}, err
	}
	perm := mode.Perm()
	if !recursive {
		if info.IsDir() && perm&0o111 == 0 {
			return ChmodData{}, newOpError(ErrInvalid, "目录必须保留执行位（x），否则无法进入：请使用 0755 之类的 mode")
		}
		return chmodOne(abs, info, perm)
	}
	items := make([]chmodItem, 0, 16)
	badDir := false
	err = filepath.WalkDir(abs, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		itemInfo, err := d.Info()
		if err != nil {
			return err
		}
		if itemInfo.Mode()&fs.ModeSymlink != 0 {
			return nil // 不跟随链接，也不改链接目标
		}
		if itemInfo.IsDir() && perm&0o111 == 0 {
			badDir = true
			return nil
		}
		items = append(items, chmodItem{path: p, mode: itemInfo.Mode().Perm()})
		return nil
	})
	if err != nil {
		return ChmodData{}, err
	}
	if badDir {
		return ChmodData{}, newOpError(ErrInvalid, "递归 chmod 的 mode 缺少执行位（x），目录将无法进入：请使用 0755 之类的 mode")
	}
	changed := 0
	for _, item := range items {
		if item.mode == perm {
			continue
		}
		if err := os.Chmod(item.path, perm); err != nil {
			return ChmodData{}, err
		}
		changed++
	}
	return ChmodData{Changed: changed}, nil
}

// chmodOne 应用单项权限，权限未变时不落系统调用。
func chmodOne(abs string, info fs.FileInfo, perm fs.FileMode) (ChmodData, error) {
	if info.Mode().Perm() == perm {
		return ChmodData{Changed: 0}, nil
	}
	if err := os.Chmod(abs, perm); err != nil {
		return ChmodData{}, err
	}
	return ChmodData{Changed: 1}, nil
}
