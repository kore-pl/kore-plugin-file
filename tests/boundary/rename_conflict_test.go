package boundary

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// renameConflictRounds 是并发抢同一目标的轮数：单轮可能天然错开（恰好一胜一负），
// 只有多轮才能把「check 与 rename 之间的窗口」逼出来；100 轮在 race 构建下 < 1s。
const renameConflictRounds = 100

// TestConcurrentRenameSameTargetNoOverwrite 两路并发 file.rename 到同一目标、overwrite=false：
//
//  1. 每轮**恰好一个**成功，另一方返回 E_CONFLICT（不能两方都报成功）；
//  2. 失败方不覆盖成功方：目标内容必须是成功方的完整载荷，失败方的源文件与内容原样保留；
//  3. 全程无 .kore-*.tmp / *.tmp 临时项残留；
//  4. 整个用例必须能过 -race（结果切片按 goroutine 下标独占写，无共享读写）。
//
// 这是 699db82 的 TOCTOU（Lstat 后 Rename 静默覆盖）回归用例：修复前每轮常出现「两方都成功」。
func TestConcurrentRenameSameTargetNoOverwrite(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for round := 0; round < renameConflictRounds; round++ {
		srcA := fmt.Sprintf("src-a-%03d.txt", round)
		srcB := fmt.Sprintf("src-b-%03d.txt", round)
		dst := fmt.Sprintf("dst-%03d.txt", round)
		payloadA := strings.Repeat(fmt.Sprintf("A%03d", round), 512)
		payloadB := strings.Repeat(fmt.Sprintf("B%03d", round), 512)
		if err := os.WriteFile(filepath.Join(root, srcA), []byte(payloadA), 0o644); err != nil {
			t.Fatalf("第 %d 轮造 %s 失败: %v", round, srcA, err)
		}
		if err := os.WriteFile(filepath.Join(root, srcB), []byte(payloadB), 0o644); err != nil {
			t.Fatalf("第 %d 轮造 %s 失败: %v", round, srcB, err)
		}

		srcs := [2]string{srcA, srcB}
		payloads := [2]string{payloadA, payloadB}
		results := make([][]byte, 2)
		perrs := make([]*pluginapi.Error, 2)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for k := 0; k < 2; k++ {
			wg.Add(1)
			go func(k int) {
				defer wg.Done()
				<-start
				params := fmt.Sprintf(`{"path":%q,"name":%q,"overwrite":false}`, srcs[k], dst)
				results[k], perrs[k] = call(t, root, "file.rename", json.RawMessage(params))
			}(k)
		}
		close(start)
		wg.Wait()

		winner := -1
		successes := 0
		for k := 0; k < 2; k++ {
			if perrs[k] == nil {
				successes++
				winner = k
				continue
			}
			if problem := errorProblem(results[k], perrs[k], pluginapi.E_CONFLICT, "目标已存在"); problem != "" {
				t.Fatalf("第 %d 轮失败方（%s）错误不符合契约: %s", round, srcs[k], problem)
			}
		}
		if successes != 1 {
			t.Fatalf("第 %d 轮并发 rename 到同一目标，期望恰好 1 个成功、1 个 E_CONFLICT，实际成功 %d 个（errA=%v errB=%v）",
				round, successes, perrs[0], perrs[1])
		}

		got, err := os.ReadFile(filepath.Join(root, dst))
		if err != nil {
			t.Fatalf("第 %d 轮读目标失败: %v", round, err)
		}
		if string(got) != payloads[winner] {
			t.Fatalf("第 %d 轮目标内容被失败方覆盖: 期望成功方 %s 的载荷（%d 字节），实际 %d 字节",
				round, srcs[winner], len(payloads[winner]), len(got))
		}
		loser := 1 - winner
		loserGot, err := os.ReadFile(filepath.Join(root, srcs[loser]))
		if err != nil {
			t.Fatalf("第 %d 轮失败方源文件 %s 不应消失: %v", round, srcs[loser], err)
		}
		if string(loserGot) != payloads[loser] {
			t.Fatalf("第 %d 轮失败方源文件 %s 内容被改动", round, srcs[loser])
		}
	}

	// 无临时项残留：修复走的是原子 rename，全程不应产生 .kore-move-*.tmp / .kore-*。
	var residue []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if name != filepath.Base(root) && (strings.HasSuffix(name, ".tmp") || strings.HasPrefix(name, ".kore-")) {
			residue = append(residue, name)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("扫描临时项失败: %v", err)
	}
	if len(residue) > 0 {
		t.Fatalf("目录内残留临时项: %v", residue)
	}
}
