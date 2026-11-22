// Package fileop 实现 file 插件的方法面：参数校验、路径约束与结构化结果。
// 本包只做逻辑校验与执行面调用，不直接读写宿主文件（骨架口径）。
package fileop

import "fmt"

// maxContent 是骨架口径下 file.write 接受的最大内容长度。
const maxContent = 1 << 20

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

// Entry 是文件列表中的一项逻辑条目。
type Entry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	SizeBytes int64  `json:"size_bytes"`
}

// Executor 是文件操作的执行面；骨架实现为 noop，测试可注入故障执行器。
type Executor interface {
	List(root, rel string) ([]Entry, error)
	Read(root, rel string) (string, error)
	Write(root, rel, content string) (bool, error)
}

// listParams / readParams / writeParams 是三个方法的显式入参形状。
type listParams struct {
	Path string `json:"path"`
}

type readParams struct {
	Path string `json:"path"`
}

type writeParams struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// ListResult / ReadResult / WriteResult 是三个方法的结构化结果。
type ListResult struct {
	Root    string  `json:"root"`
	Entries []Entry `json:"entries"`
	Count   int     `json:"count"`
}

type ReadResult struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	SizeBytes int    `json:"size_bytes"`
	Encoding  string `json:"encoding"`
}

type WriteResult struct {
	Path         string `json:"path"`
	BytesWritten int    `json:"bytes_written"`
	Created      bool   `json:"created"`
	Applied      bool   `json:"applied"`
}
