package fileop

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// previewRunes 是内容命中行预览的最大字符数。
const previewRunes = 120

// Search 遍历搜索根：文件名按 glob 匹配，content 非空时再做有界内容搜索。
// 约束：不跟随符号链接、不进入保留的回收站目录、单次最多遍历 maxSearchScanEntries 条，
// 内容搜索只读 size ≤ MaxFileBytes 的常规文件，含 NUL 的二进制文件直接跳过。
func (realFS) Search(req SearchRequest) (SearchData, error) {
	rootInfo, err := os.Stat(req.RootAbs)
	if err != nil {
		return SearchData{}, err
	}
	if !rootInfo.IsDir() {
		return SearchData{}, newOpError(ErrInvalid, "搜索根不是目录，请传目录路径")
	}
	out := SearchData{Entries: []SearchMatch{}}
	walkErr := filepath.WalkDir(req.RootAbs, func(abs string, d fs.DirEntry, err error) error {
		if err != nil {
			out.Skipped++
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if abs == req.RootAbs {
			return nil
		}
		rel, relErr := filepath.Rel(req.RootAbs, abs)
		if relErr != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		if relSlash == TrashDirName {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if out.Scanned >= maxSearchScanEntries {
			out.Truncated = true
			return fs.SkipAll
		}
		out.Scanned++
		info, infoErr := d.Info()
		if infoErr != nil {
			out.Skipped++
			return nil
		}
		isSymlink := d.Type()&fs.ModeSymlink != 0
		if !globMatch(req.Pattern, relSlash, d.Name(), req.IgnoreCase) {
			return nil
		}
		// 内容搜索是名称 glob 之上的二次过滤：名字不匹配的文件根本不打开，
		// 名字匹配但超体积 / 二进制 / 特殊文件的计入 skipped 且不算命中。
		line, preview := 0, ""
		if req.Content != "" {
			hit, hitPreview, skip := searchContent(abs, info, isSymlink, req)
			if skip {
				out.Skipped++
			}
			if hit == 0 {
				return nil
			}
			line, preview = hit, hitPreview
		}
		out.Entries = append(out.Entries, SearchMatch{
			Path: relSlash, Name: d.Name(), IsDir: d.IsDir(),
			Size: info.Size(), Mode: modeString(uint32(info.Mode().Perm())),
			ModTime: formatSearchModTime(info.ModTime()), IsSymlink: isSymlink,
			Line: line, Preview: preview,
		})
		if len(out.Entries) >= req.MaxResults {
			out.Truncated = true
			return fs.SkipAll
		}
		return nil
	})
	if walkErr != nil {
		return SearchData{}, walkErr
	}
	out.Matched = len(out.Entries)
	return out, nil
}

// globMatch 按文件名匹配；pattern 含 "/" 时改按根相对路径匹配。
func globMatch(pattern, relSlash, name string, ignoreCase bool) bool {
	target := name
	if strings.Contains(pattern, "/") {
		target = relSlash
	}
	if ignoreCase {
		pattern = strings.ToLower(pattern)
		target = strings.ToLower(target)
	}
	ok, err := path.Match(pattern, target)
	return err == nil && ok
}

// searchContent 在单个文件里找关键字：返回 1 基行号；skip 表示因超体积/二进制/特殊文件跳过。
func searchContent(abs string, info fs.FileInfo, isSymlink bool, req SearchRequest) (int, string, bool) {
	if isSymlink || !info.Mode().IsRegular() {
		return 0, "", true
	}
	if info.Size() > req.MaxFileBytes {
		return 0, "", true
	}
	file, err := os.Open(abs)
	if err != nil {
		return 0, "", true
	}
	defer func() { _ = file.Close() }()
	head := make([]byte, 8<<10)
	n, readErr := file.Read(head)
	if readErr != nil && readErr != io.EOF {
		return 0, "", true
	}
	if bytes.IndexByte(head[:n], 0) >= 0 {
		return 0, "", true
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, "", true
	}
	content, err := io.ReadAll(io.LimitReader(file, req.MaxFileBytes+1))
	if err != nil || int64(len(content)) > req.MaxFileBytes {
		return 0, "", true
	}
	return matchContentLine(string(content), req.Content, req.IgnoreCase)
}

// matchContentLine 逐行比对（大小写折叠用同一行的 ToLower 结果），返回行号与脱敏预览。
func matchContentLine(content, needle string, ignoreCase bool) (int, string, bool) {
	lowerNeedle := needle
	if ignoreCase {
		lowerNeedle = strings.ToLower(needle)
	}
	for index, line := range strings.Split(content, "\n") {
		candidate := line
		if ignoreCase {
			candidate = strings.ToLower(line)
		}
		if strings.Contains(candidate, lowerNeedle) {
			return index + 1, sanitizePreview(line), false
		}
	}
	return 0, "", false
}

// sanitizePreview 去首尾空白、把控制字符替换为空格，并截断到 previewRunes 个字符。
func sanitizePreview(line string) string {
	clean := strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, line)
	clean = strings.TrimSpace(clean)
	runes := []rune(clean)
	if len(runes) > previewRunes {
		return string(runes[:previewRunes]) + "…"
	}
	return clean
}
