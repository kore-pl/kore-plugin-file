package fileop

import (
	"encoding/json"
	"fmt"
	"path"
	"time"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// searchParams 是 file.search 的显式入参形状。
// pattern 是必填的文件名 glob；content 非空时再做内容搜索（受 max_file_bytes 限制）。
type searchParams struct {
	Path         string `json:"path"`
	Pattern      string `json:"pattern"`
	Content      string `json:"content"`
	IgnoreCase   bool   `json:"ignore_case"`
	MaxResults   int    `json:"max_results"`
	MaxFileBytes int64  `json:"max_file_bytes"`
}

// SearchMatch 是一条命中：文件名命中时 line=0，内容命中时带行号与行预览。
type SearchMatch struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	IsDir     bool   `json:"is_dir"`
	Size      int64  `json:"size"`
	Mode      string `json:"mode"`
	ModTime   string `json:"mod_time"`
	IsSymlink bool   `json:"is_symlink"`
	Line      int    `json:"line"`
	Preview   string `json:"preview"`
}

// SearchResult 是 file.search 的结果；truncated 表示因结果上限或遍历上限提前停止，
// skipped 表示内容搜索时被跳过（超体积、二进制、特殊文件）的条目数。
type SearchResult struct {
	Path      string        `json:"path"`
	Pattern   string        `json:"pattern"`
	Content   string        `json:"content"`
	Scanned   int           `json:"scanned"`
	Matched   int           `json:"matched"`
	Skipped   int           `json:"skipped"`
	Truncated bool          `json:"truncated"`
	Entries   []SearchMatch `json:"entries"`
}

// SearchRequest 是 file.search 执行面的完整请求（路径与阈值都已校验）。
type SearchRequest struct {
	RootAbs      string
	Pattern      string
	Content      string
	IgnoreCase   bool
	MaxResults   int
	MaxFileBytes int64
}

// SearchData 是 file.search 执行面返回的原始结果。
type SearchData struct {
	Scanned   int
	Matched   int
	Skipped   int
	Truncated bool
	Entries   []SearchMatch
}

// callSearch 处理 file.search：先校验 glob / 关键词与阈值，再交给执行面遍历。
// 搜索根走 ResolveExisting，遍历不跟随符号链接，且跳过保留的回收站目录。
func callSearch(raw json.RawMessage, root string, exec Executor) (json.RawMessage, *pluginapi.Error) {
	var p searchParams
	if perr := decodeParams(raw, &p); perr != nil {
		return nil, perr
	}
	rootRaw := p.Path
	if rootRaw == "" {
		rootRaw = "."
	}
	display := displayPath(rootRaw)
	abs, perr := ResolveExisting(root, rootRaw)
	if perr != nil {
		return nil, perr
	}
	if perr := validateSearchPattern(p.Pattern); perr != nil {
		return nil, perr
	}
	if len(p.Content) > MaxSearchNeedleBytes {
		return nil, invalid(fmt.Sprintf("content 超过 %d 字节上限", MaxSearchNeedleBytes))
	}
	maxResults, perr := effectiveSearchResults(p.MaxResults)
	if perr != nil {
		return nil, perr
	}
	maxFileBytes, perr := effectiveSearchContentMax(p.MaxFileBytes)
	if perr != nil {
		return nil, perr
	}
	data, err := exec.Search(SearchRequest{
		RootAbs: abs, Pattern: p.Pattern, Content: p.Content,
		IgnoreCase: p.IgnoreCase, MaxResults: maxResults, MaxFileBytes: maxFileBytes,
	})
	if err != nil {
		return nil, classify(err)
	}
	entries := data.Entries
	if entries == nil {
		entries = []SearchMatch{}
	}
	return encode(SearchResult{
		Path: display, Pattern: p.Pattern, Content: p.Content,
		Scanned: data.Scanned, Matched: len(entries), Skipped: data.Skipped,
		Truncated: data.Truncated, Entries: entries,
	})
}

// validateSearchPattern 要求非空且是合法 glob；非法模式提前拒绝，避免遍历中途才发现。
func validateSearchPattern(pattern string) *pluginapi.Error {
	if pattern == "" {
		return invalid("pattern 不能为空，请传文件名 glob，例如 \"*.log\"")
	}
	if len(pattern) > MaxSearchPatternBytes {
		return invalid(fmt.Sprintf("pattern 超过 %d 字节上限", MaxSearchPatternBytes))
	}
	if _, err := path.Match(pattern, "x"); err != nil {
		return invalid("pattern 不是合法的 glob 表达式: " + err.Error())
	}
	return nil
}

// effectiveSearchResults 计算结果上限：缺省 200，显式值 1..1000。
func effectiveSearchResults(requested int) (int, *pluginapi.Error) {
	if requested == 0 {
		return DefaultSearchResults, nil
	}
	if requested < 1 || requested > MaxSearchResults {
		return 0, invalid(fmt.Sprintf("max_results 必须在 1 到 %d 之间", MaxSearchResults))
	}
	return requested, nil
}

// effectiveSearchContentMax 计算单文件内容读取上限：缺省 1 MiB，显式值 1..8 MiB。
func effectiveSearchContentMax(requested int64) (int64, *pluginapi.Error) {
	if requested == 0 {
		return DefaultSearchContentMax, nil
	}
	if requested < 1 || requested > MaxSearchContentMax {
		return 0, invalid(fmt.Sprintf("max_file_bytes 必须在 1 到 %d 之间", MaxSearchContentMax))
	}
	return requested, nil
}

// formatSearchModTime 把修改时间格式化为 RFC3339，空时间回空串。
func formatSearchModTime(mtime time.Time) string {
	if mtime.IsZero() {
		return ""
	}
	return mtime.Format(time.RFC3339)
}
