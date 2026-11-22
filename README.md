# kore-plugin-file

文件管理插件（**真实读写**）：在受控根目录内提供文件列表、读取、原子写入、删除、建目录与元信息查询。不内置 unzip/tar/chmod 等工具二进制（裁决 37 / 提案 §2.1 批 1）。

## 协议面

- `kore.plugin.v1`，`lifecycle=lazy`，`depth_level=production`
- 能力声明：`system:read` + `system:admin`（真实写盘需要管理能力）
- 方法（同时接受裸 `list` / `read` / `write` / `delete` / `mkdir` / `stat`）：

| 方法 | 入参 | 结果要点 |
| --- | --- | --- |
| `file.list` | `{path}` | `entries[]`：`name` / `path` / `is_dir` / `size` / `mode` / `mod_time`(RFC3339) / `is_symlink` |
| `file.read` | `{path, max_bytes?}` | `content`（UTF-8）/ `size` / `truncated` / `is_binary`；含 NUL 判二进制，不回 `content` |
| `file.write` | `{path, content, create_if_missing?}` | `size` / `mod_time` / `created`；临时文件 + `rename` 原子替换 |
| `file.delete` | `{path, recursive?}` | `deleted`；非空目录且 `recursive=false` 回 `E_CONFLICT` |
| `file.mkdir` | `{path, mode?}` | `mode`（默认 `0755`，八进制字符串）/ `created` |
| `file.stat` | `{path}` | `size` / `mode` / `owner{uid,gid}` / `mod_time` / `is_dir` / `is_symlink` |

- 单次上限：`read` 默认 1 MiB（`max_bytes` 可调到 8 MiB 硬上限），`write` 8 MiB
- 握手的 stdout 留给协议帧，日志一律写 stderr

## 安全约束

所有路径限制在配置根目录（`KORE_FILE_ROOT`，缺省 `/tmp/kore-file-root`）之内，两道校验：

1. **词法校验**：拒绝空路径、绝对路径、`..` 段、反斜杠与 NUL；`filepath.Clean` 后用 `filepath.Rel` 判定仍在根内。
2. **符号链接二次校验**：`filepath.EvalSymlinks` 解析后再次用 `filepath.Rel` 校验真实路径；写入 / 建目录时解析最近已存在祖先，杜绝“父目录是外链”的逃逸。

拒绝一律返回 `E_INVALID` 并说明原因，错误信息不回显宿主绝对路径；根目录本身禁止删除。资源不存在返回 `E_NOTFOUND`，状态冲突返回 `E_CONFLICT`。

## 四类故障注入

`tests/fault/` 四个文件分别注入 kill -9、超时、磁盘满、cgroup 超限，断言方法级返回结构化错误，且 `Error.Code` 与故障类别一一对应：

| 故障 | 文件 | 期望错误码 |
| --- | --- | --- |
| kill -9 | `kill_test.go` | `E_UPSTREAM` |
| 超时 | `timeout_test.go` | `E_UPSTREAM` |
| 磁盘满 | `disk_test.go` | `E_INTERNAL` |
| cgroup 超限 | `cgroup_test.go` | `E_UNSUPPORTED` |

## 构建与验收

```bash
gofmt -l . && go vet ./... && go test -timeout 200s ./...
make build && make dist
tar -tzf dist/file-0.1.0.tar.gz | head -1   # info.json
```

`dist` 使用确定性参数归档（`--sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner` + `gzip -9n`），重复构建 sha256 不变；`size_bytes` 由打包脚本回填，与实际包体偏差 ≤10%。

## 本机端到端

插件以内核客户端身份连接 UDS。没有内核时可用临时假内核完成握手后直接调用；`KORE_FILE_ROOT` 指向临时目录：

```bash
KORE_PLUGIN_SOCK=/tmp/kore-file-e2e.sock KORE_FILE_ROOT=/tmp/kore-file-e2e-init \
  ./bin/file
```

真实读写、原子性、符号链接逃逸拒绝与六方法 JSON 返回均由 `internal/fileop` 单测（`t.TempDir()` 内真实落盘）覆盖。
