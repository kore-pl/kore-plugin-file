package boundary

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/pengtang1024/kore-plugin-file/internal/fileop"
)

// 并发边界用例的共同口径：
//   - 全部走真实文件系统 + 多 goroutine，配合 `go test -race` 检测数据竞争；
//   - 断言的是**方法面契约**（内容原子性、无路径串扰、无错误），不是内部实现；
//   - 每个用例自带独立 root，用例之间用 t.Parallel 真并行。

const (
	hotFileSize   = 32 * 1024 // 单次写入内容大小；足以让非原子写暴露撕裂
	hotRounds     = 30        // 每个 writer/reader 的轮数
	hotWriters    = 4
	hotReaders    = 4
	renameWorkers = 8
	rootWorkers   = 6
	rootRounds    = 12
)

// TestConcurrentReadWriteSameFile 同一文件被多 writer 原子重写、多 reader 并发读取：
// 每次读到的内容必须是某一次完整写入（全 A 或全 B），不能出现空内容 / 截断 / 混合内容；
// 最终文件内容也必须是两个完整载荷之一。
func TestConcurrentReadWriteSameFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	payloadA := strings.Repeat("A", hotFileSize)
	payloadB := strings.Repeat("B", hotFileSize)
	if err := os.WriteFile(filepath.Join(root, "hot.txt"), []byte(payloadA), 0o644); err != nil {
		t.Fatalf("造初始文件失败: %v", err)
	}

	problems := make(chan string, (hotWriters+hotReaders)*hotRounds)
	var wg sync.WaitGroup
	for w := 0; w < hotWriters; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for round := 0; round < hotRounds; round++ {
				payload := payloadA
				if (id+round)%2 == 1 {
					payload = payloadB
				}
				_, perr := fileop.Call("file.write", writeJSON("hot.txt", payload), root, nil)
				if problem := successProblem(perr); problem != "" {
					problems <- fmt.Sprintf("writer %d 第 %d 轮写入失败: %s", id, round, problem)
					return
				}
			}
		}(w)
	}
	for r := 0; r < hotReaders; r++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for round := 0; round < hotRounds; round++ {
				result, perr := fileop.Call("file.read", json.RawMessage(`{"path":"hot.txt"}`), root, nil)
				if problem := successProblem(perr); problem != "" {
					problems <- fmt.Sprintf("reader %d 第 %d 轮读取失败: %s", id, round, problem)
					return
				}
				var out fileop.ReadResult
				if err := json.Unmarshal(result, &out); err != nil {
					problems <- fmt.Sprintf("reader %d 第 %d 轮结果解析失败: %v", id, round, err)
					return
				}
				if out.Content == nil {
					problems <- fmt.Sprintf("reader %d 第 %d 轮 content 为空（size=%d truncated=%v）", id, round, out.Size, out.Truncated)
					return
				}
				if out.Size != hotFileSize || *out.Content != payloadA && *out.Content != payloadB {
					problems <- fmt.Sprintf("reader %d 第 %d 轮读到撕裂内容: size=%d（期望 %d，且内容必须是完整载荷）", id, round, out.Size, hotFileSize)
					return
				}
			}
		}(r)
	}
	wg.Wait()
	close(problems)
	failures := 0
	for problem := range problems {
		t.Error(problem)
		failures++
	}
	if failures > 0 {
		t.FailNow()
	}

	// 收尾再读一次：并发结束后文件必须是完整载荷，不能停在中间状态。
	final := struct {
		Content *string `json:"content"`
		Size    int64   `json:"size"`
	}{}
	var raw struct {
		Content *string `json:"content"`
		Size    int64   `json:"size"`
	}
	data := mustCall(t, root, "file.read", json.RawMessage(`{"path":"hot.txt"}`))
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("收尾读取结果解析失败: %v", err)
	}
	if raw.Content == nil || (*raw.Content != payloadA && *raw.Content != payloadB) || raw.Size != hotFileSize {
		t.Fatalf("并发结束后文件不是完整载荷: size=%v contentNil=%v", raw.Size, raw.Content == nil)
	}
	_ = final
}

// TestConcurrentRenameDistinctTargets 多个 rename 并发改名各自文件，同时有 lister 持续列目录：
// lister 只能看到合法条目名（src-N / dst-N），rename 全部成功，最终目录恰为 dst-N 集合，
// 且每个 dst-N 的内容与它对应的载荷一致（改名不能把内容串到别的条目）。
func TestConcurrentRenameDistinctTargets(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	payloads := make([]string, renameWorkers)
	for i := 0; i < renameWorkers; i++ {
		payloads[i] = strings.Repeat(fmt.Sprintf("%d", i), 4096)
		name := fmt.Sprintf("src-%d.txt", i)
		if err := os.WriteFile(filepath.Join(root, name), []byte(payloads[i]), 0o644); err != nil {
			t.Fatalf("造 %s 失败: %v", name, err)
		}
	}

	entryName := regexp.MustCompile(`^(src|dst)-\d+\.txt$`)
	problems := make(chan string, renameWorkers*4+64)
	stop := make(chan struct{})

	var lister sync.WaitGroup
	lister.Add(1)
	go func() {
		defer lister.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			result, perr := fileop.Call("file.list", json.RawMessage(`{"path":"."}`), root, nil)
			if problem := successProblem(perr); problem != "" {
				problems <- "并发 list 失败: " + problem
				return
			}
			var out fileop.ListResult
			if err := json.Unmarshal(result, &out); err != nil {
				problems <- "并发 list 结果解析失败: " + err.Error()
				return
			}
			for _, entry := range out.Entries {
				if !entryName.MatchString(entry.Name) {
					problems <- fmt.Sprintf("list 看到非法条目名 %q（改名/列目录出现串扰或临时文件残留）", entry.Name)
					return
				}
			}
		}
	}()

	var renamers sync.WaitGroup
	for i := 0; i < renameWorkers; i++ {
		renamers.Add(1)
		go func(id int) {
			defer renamers.Done()
			params, err := json.Marshal(struct {
				Path string `json:"path"`
				Name string `json:"name"`
			}{Path: fmt.Sprintf("src-%d.txt", id), Name: fmt.Sprintf("dst-%d.txt", id)})
			if err != nil {
				problems <- "rename 参数序列化失败: " + err.Error()
				return
			}
			_, perr := fileop.Call("file.rename", params, root, nil)
			if problem := successProblem(perr); problem != "" {
				problems <- fmt.Sprintf("rename worker %d 失败: %s", id, problem)
			}
		}(i)
	}
	renamers.Wait()
	close(stop)
	lister.Wait()
	close(problems)
	failures := 0
	for problem := range problems {
		t.Error(problem)
		failures++
	}
	if failures > 0 {
		t.FailNow()
	}

	data := mustCall(t, root, "file.list", json.RawMessage(`{"path":"."}`))
	var listing fileop.ListResult
	if err := json.Unmarshal(data, &listing); err != nil {
		t.Fatalf("收尾 list 解析失败: %v", err)
	}
	if len(listing.Entries) != renameWorkers {
		t.Fatalf("收尾目录条目数=%d，期望 %d", len(listing.Entries), renameWorkers)
	}
	for i := 0; i < renameWorkers; i++ {
		name := fmt.Sprintf("dst-%d.txt", i)
		readResult := mustCall(t, root, "file.read", json.RawMessage(fmt.Sprintf(`{"path":%q}`, name)))
		var out fileop.ReadResult
		if err := json.Unmarshal(readResult, &out); err != nil {
			t.Fatalf("读 %s 解析失败: %v", name, err)
		}
		if out.Content == nil || *out.Content != payloads[i] {
			t.Fatalf("%s 内容与源载荷不一致（改名过程出现状态串扰）", name)
		}
	}
}

// TestConcurrentIndependentRootsNoCrosstalk 每个 goroutine 持有自己的 root，
// 并发执行 write → read → rename → read：读回内容必须恒等于自己写的载荷，
// 不能读到别的 root 的内容（验证调用面没有跨 root 的共享状态）。
func TestConcurrentIndependentRootsNoCrosstalk(t *testing.T) {
	t.Parallel()

	roots := make([]string, rootWorkers)
	for i := range roots {
		roots[i] = t.TempDir()
	}

	problems := make(chan string, rootWorkers*rootRounds*2)
	var wg sync.WaitGroup
	for w := 0; w < rootWorkers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			root := roots[id]
			for round := 0; round < rootRounds; round++ {
				payload := strings.Repeat(fmt.Sprintf("w%d-r%d-", id, round), 512)
				name := fmt.Sprintf("f-%d-%d.txt", id, round)
				renamed := fmt.Sprintf("g-%d-%d.txt", id, round)
				if _, perr := fileop.Call("file.write", writeJSON(name, payload), root, nil); perr != nil {
					problems <- fmt.Sprintf("worker %d 第 %d 轮写入失败: %s: %s", id, round, perr.Code, perr.Message)
					return
				}
				verify := func(path string) bool {
					result, perr := fileop.Call("file.read", json.RawMessage(fmt.Sprintf(`{"path":%q}`, path)), root, nil)
					if perr != nil {
						problems <- fmt.Sprintf("worker %d 第 %d 轮读 %s 失败: %s: %s", id, round, path, perr.Code, perr.Message)
						return false
					}
					var out fileop.ReadResult
					if err := json.Unmarshal(result, &out); err != nil {
						problems <- fmt.Sprintf("worker %d 第 %d 轮读 %s 解析失败: %v", id, round, path, err)
						return false
					}
					if out.Content == nil || *out.Content != payload {
						problems <- fmt.Sprintf("worker %d 第 %d 轮读 %s 内容与写入不一致（跨 root 状态串扰）", id, round, path)
						return false
					}
					return true
				}
				if !verify(name) {
					return
				}
				params, err := json.Marshal(struct {
					Path string `json:"path"`
					Name string `json:"name"`
				}{Path: name, Name: renamed})
				if err != nil {
					problems <- "rename 参数序列化失败: " + err.Error()
					return
				}
				if _, perr := fileop.Call("file.rename", params, root, nil); perr != nil {
					problems <- fmt.Sprintf("worker %d 第 %d 轮改名失败: %s: %s", id, round, perr.Code, perr.Message)
					return
				}
				if !verify(renamed) {
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(problems)
	for problem := range problems {
		t.Error(problem)
	}
}
