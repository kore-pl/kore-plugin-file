package fileop

import "regexp"

// trashIDPattern 限定回收站条目 id：<19 位纳秒>-<8 位十六进制>，只含安全字符。
var trashIDPattern = regexp.MustCompile(`^[0-9]{19}-[0-9a-f]{8}$`)

// TrashEntry 是回收站条目：既作为 meta.json 的落盘形状，也作为对外结果（G-24 显式字段）。
type TrashEntry struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Size      int64  `json:"size"`
	DeletedAt string `json:"deleted_at"`
}

// TrashListing 是执行面列目录的原始结果。
type TrashListing struct {
	Entries   []TrashEntry
	Total     int
	Skipped   int
	Truncated bool
}

// trashListParams 是 file.trash.list 的显式入参形状。
type trashListParams struct {
	Limit int `json:"limit"`
}

// TrashListResult 是 file.trash.list 的结果。
type TrashListResult struct {
	Count     int          `json:"count"`
	Total     int          `json:"total"`
	Skipped   int          `json:"skipped"`
	Truncated bool         `json:"truncated"`
	Entries   []TrashEntry `json:"entries"`
}

// trashRestoreParams 是 file.trash.restore 的显式入参形状。
type trashRestoreParams struct {
	ID        string `json:"id"`
	Overwrite bool   `json:"overwrite"`
}

// TrashRestoreResult 是 file.trash.restore 的结果。
type TrashRestoreResult struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	Restored bool   `json:"restored"`
}

// trashPurgeParams 是 file.trash.purge 的显式入参形状：按 id 清一条，或 all=true 清空。
type trashPurgeParams struct {
	ID  string `json:"id"`
	All bool   `json:"all"`
}

// TrashPurgeResult 是 file.trash.purge 的结果：purged 是实际删除的条目数。
type TrashPurgeResult struct {
	Purged int  `json:"purged"`
	All    bool `json:"all"`
}
