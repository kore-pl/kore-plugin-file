// Package fileop 实现 file 插件的方法面：真实文件操作、路径约束与结构化结果。
//
// 安全基线：路径先做词法校验（filepath.Clean + filepath.Rel），再在解析符号链接后
// 做一次根目录归属校验；执行面只接收通过校验的绝对路径，结果只回相对路径，
// 不把宿主绝对路径暴露给调用方。
package fileop

import (
	"fmt"
	"io/fs"
	"time"
)

const (
	// DefaultReadLimit 是 file.read 未指定 max_bytes 时的默认读取上限：1 MiB。
	DefaultReadLimit int64 = 1 << 20
	// MaxReadBytes 是 file.read 的硬上限：显式 max_bytes 最大可调到 8 MiB。
	MaxReadBytes int64 = 8 << 20
	// MaxWriteBytes 是 file.write 单次内容上限：8 MiB。
	MaxWriteBytes = 8 << 20
	// DefaultDirMode 与 DefaultFileMode 是新建目录 / 文件的缺省权限。
	DefaultDirMode  = fs.FileMode(0o755)
	DefaultFileMode = fs.FileMode(0o644)
)

// FaultKind 标识一次故障注入的类别。
type FaultKind string

const (
	FaultNone    FaultKind = ""
	FaultKill    FaultKind = "kill"
	FaultTimeout FaultKind = "timeout"
	FaultDisk    FaultKind = "disk"
	FaultCgroup  FaultKind = "cgroup"
)

// FaultError 是执行面注入的故障错误，Call 将其映射为协议错误码。
type FaultError struct {
	Kind    FaultKind
	Message string
}

// Error 实现 error 接口，输出 "<类别>: <说明>"。
func (e *FaultError) Error() string { return fmt.Sprintf("%s: %s", e.Kind, e.Message) }

// NewFaultError 构造给定类别的故障错误。
func NewFaultError(kind FaultKind, message string) *FaultError {
	return &FaultError{Kind: kind, Message: message}
}

// ErrKind 是执行面业务错误的语义类别，classify 将其映射为协议错误码。
type ErrKind string

const (
	ErrInvalid     ErrKind = "invalid"
	ErrNotFound    ErrKind = "notfound"
	ErrConflict    ErrKind = "conflict"
	ErrInternal    ErrKind = "internal"
	ErrUnsupported ErrKind = "unsupported"
)

// OpError 是执行面返回的可分类业务错误；Message 不得包含宿主绝对路径。
type OpError struct {
	Kind    ErrKind
	Message string
}

// Error 实现 error 接口。
func (e *OpError) Error() string { return string(e.Kind) + ": " + e.Message }

// newOpError 构造 OpError。
func newOpError(kind ErrKind, message string) *OpError {
	return &OpError{Kind: kind, Message: message}
}

// Entry 是目录列表中的一项（字段为文件系统原始类型，编码时再格式化）。
type Entry struct {
	Name      string
	IsDir     bool
	Size      int64
	Mode      fs.FileMode
	ModTime   time.Time
	IsSymlink bool
}

// ReadData 是 file.read 执行面返回的原始结果。
type ReadData struct {
	Content   []byte
	Size      int64
	Truncated bool
	Binary    bool
}

// WriteData 是 file.write 执行面返回的写后元信息。
type WriteData struct {
	Size    int64
	ModTime time.Time
	Created bool
}

// StatData 是 file.stat 执行面返回的详细元信息。
type StatData struct {
	Size      int64
	Mode      fs.FileMode
	ModTime   time.Time
	IsDir     bool
	IsSymlink bool
	UID       uint32
	GID       uint32
}

// Executor 是文件操作的执行面；nil 时使用真实文件系统实现。
type Executor interface {
	List(abs string) ([]Entry, error)
	Read(abs string, limit int64) (ReadData, error)
	Write(abs string, content string, createIfMissing bool) (WriteData, error)
	Delete(abs string, recursive bool) error
	Mkdir(abs string, mode fs.FileMode) error
	Stat(abs string) (StatData, error)
}
