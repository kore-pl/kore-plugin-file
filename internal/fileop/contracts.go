package fileop

// 本文件是方法面的显式契约：入参形状与结果形状，禁止使用 object / any 逃逸。

// listParams / readParams / writeParams / deleteParams / mkdirParams / statParams
// 是六个方法的显式入参形状。
type listParams struct {
	Path string `json:"path"`
}

type readParams struct {
	Path     string `json:"path"`
	MaxBytes int64  `json:"max_bytes"`
}

type writeParams struct {
	Path            string `json:"path"`
	Content         string `json:"content"`
	CreateIfMissing bool   `json:"create_if_missing"`
}

type deleteParams struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"`
	// Permanent 为 true 时直接删除（旧语义）；缺省 false 表示移入回收站。
	Permanent bool `json:"permanent"`
}

type mkdirParams struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
}

type statParams struct {
	Path string `json:"path"`
}

// ListEntry 是目录条目对外的显式形状。
type ListEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	IsDir     bool   `json:"is_dir"`
	Size      int64  `json:"size"`
	Mode      string `json:"mode"`
	ModTime   string `json:"mod_time"`
	IsSymlink bool   `json:"is_symlink"`
	// UID/GID 供文件列表展示属主；非 unix 平台恒为 0。
	UID uint32 `json:"uid"`
	GID uint32 `json:"gid"`
}

// ListResult 是 file.list 的结果。
type ListResult struct {
	Path    string      `json:"path"`
	Entries []ListEntry `json:"entries"`
	Count   int         `json:"count"`
}

// ReadResult 是 file.read 的结果；二进制文件省略 content。
// 当前实现超过上限即拒绝（提示 file.stat），truncated 恒为 false，为流式读取预留。
type ReadResult struct {
	Path      string  `json:"path"`
	Content   *string `json:"content,omitempty"`
	Size      int64   `json:"size"`
	Truncated bool    `json:"truncated"`
	IsBinary  bool    `json:"is_binary"`
}

// WriteResult 是 file.write 的结果。
type WriteResult struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
	Created bool   `json:"created"`
}

// DeleteResult 是 file.delete 的结果；permanent=false 时 trash_id 指向回收站条目。
type DeleteResult struct {
	Path      string `json:"path"`
	Deleted   bool   `json:"deleted"`
	Permanent bool   `json:"permanent"`
	TrashID   string `json:"trash_id,omitempty"`
}

// MkdirResult 是 file.mkdir 的结果。
type MkdirResult struct {
	Path    string `json:"path"`
	Mode    string `json:"mode"`
	Created bool   `json:"created"`
}

// Owner 是文件属主（uid/gid）。
type Owner struct {
	UID uint32 `json:"uid"`
	GID uint32 `json:"gid"`
}

// StatResult 是 file.stat 的结果。
type StatResult struct {
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	Mode      string `json:"mode"`
	Owner     Owner  `json:"owner"`
	ModTime   string `json:"mod_time"`
	IsDir     bool   `json:"is_dir"`
	IsSymlink bool   `json:"is_symlink"`
}
