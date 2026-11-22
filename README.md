# kore-plugin-file

文件管理插件（**L2 可用**）：在受控根目录内提供 12 个真实文件方法——列表、读取、原子写入、删除、建目录、元信息、上传、下载、解压、压缩、改权限、移动/改名。不内置 unzip/tar/chmod 等工具二进制（裁决 37），归档能力全部走 Go 标准库。

## 协议面

- `kore.plugin.v1`，`lifecycle=lazy`，`depth_level=production`
- 能力声明：`system:read` + `system:admin`（真实写盘需要管理能力）
- 方法名一律 `file.<verb>`；裸名（`list` / `read` / `write` / `delete` / `mkdir` / `stat`）仅为历史兼容，L2 新方法必须带 `file.` 前缀

| 方法 | 入参 | 结果要点 |
| --- | --- | --- |
| `file.list` | `{path}` | `entries[]`：`name` / `path` / `is_dir` / `size` / `mode` / `mod_time`(RFC3339) / `is_symlink` |
| `file.read` | `{path, max_bytes?}` | `content`（UTF-8）/ `size` / `truncated` / `is_binary`；含 NUL 判二进制，不回 `content` |
| `file.write` | `{path, content, create_if_missing?}` | `size` / `mod_time` / `created`；临时文件 + `rename` 原子替换 |
| `file.delete` | `{path, recursive?}` | `deleted`；非空目录且 `recursive=false` 回 `E_CONFLICT` |
| `file.mkdir` | `{path, mode?}` | `mode`（默认 `0755`，八进制字符串）/ `created` |
| `file.stat` | `{path}` | `size` / `mode` / `owner{uid,gid}` / `mod_time` / `is_dir` / `is_symlink` |
| `file.upload` | `{path, filename, data_base64, overwrite?}` | `path` / `filename` / `size` / `sha256` / `created`；同名且 `overwrite=false` 回 `E_CONFLICT`；原子替换并继承已有权限位 |
| `file.download` | `{path}` | `filename` / `size_bytes` / `data_base64`；超限回 `E_INVALID` 并提示“文件过大，请用目录打包下载” |
| `file.extract` | `{archive_path, dest_dir, overwrite?}` | `extracted` / `skipped[]` / `bytes` / `format`；支持 zip / tar / tar.gz / tgz；`overwrite=false` 遇同名跳过并记入 `skipped[]` |
| `file.compress` | `{paths[], archive_path, format?}` | `archive_path` / `size_bytes` / `entries` / `format`（`zip` 缺省，或 `tar.gz`）；递归目录、保留相对路径、不跟随符号链接 |
| `file.chmod` | `{path, mode, recursive?}` | `path` / `mode` / `changed`；`mode` 是八进制字符串；目录缺执行位回 `E_INVALID`，递归先整树校验再统一应用 |
| `file.move` | `{src, dst, overwrite?}` | `from` / `to` / `moved` / `method`；`method=rename` 或跨文件系统回退 `copy+delete`；目标已存在且 `overwrite=false` 回 `E_CONFLICT` |

### 传输上限（口径必须说清）

`file.upload` / `file.download` 的方法契约上限是 **32 MiB**（`MaxTransferBytes`）。但协议单帧硬上限是 **8 MiB**（`pluginapi.MaxFrameSize`），base64 编码膨胀 4/3，所以单次请求实际可承载 **≈5.95 MiB**（`MaxInlineTransferBytes = (8 MiB - 64 KiB) / 4 * 3`）。超过即回 `E_INVALID`：

- 上传提示单帧上限、base64 膨胀、方法契约上限，并说明**大文件分片上传在 L3 提供**
- 下载提示“文件过大，请用目录打包下载”，并说明分片下载在 L3 提供

不把 32 MiB 写成“现在就能走通”的假口径；分片协议落地前，真实上限就是单帧可承载量。

## 归档安全：解压防穿越（对照 backup 的六层）

`file.extract` 对每个条目名做与 `validateRestoreHeader` 等价的六层校验，任何一条不满足**整包中止**：

1. 条目名非空
2. 长度 ≤ 4096 字节
3. 不含 NUL / 反斜杠
4. 不是绝对路径
5. 不含 `..` 段
6. `path.Clean` 后仍非 `.`、非空、非 `../` 前缀

在此之上再加四道执行层校验：

- **类型白名单**：只接受目录、常规文件、符号链接；字符/块设备、FIFO、硬链接整包拒绝（zip 另拒“目录条目带内容”）
- **单条目大小**：常规文件 ≤ 2 GiB
- **符号链接目标**：不得绝对路径、不得含 NUL/反斜杠、解析后不得越出解压目录
- **逐条目路径归属**：每个条目落盘前再走一次与 `file.read`/`file.write` 完全相同的 `ResolveTarget`，劫持父目录的符号链接同样被拒

解压采用**先整包校验、后落盘**两遍：校验不通过时目标目录不会被创建，恶意包零残留。

## 路径安全约束

所有路径限制在配置根目录（`KORE_FILE_ROOT`，缺省 `/tmp/kore-file-root`）之内，两道校验：

1. **词法校验**：拒绝空路径、绝对路径、`..` 段、反斜杠与 NUL；`filepath.Clean` 后用 `filepath.Rel` 判定仍在根内。
2. **符号链接二次校验**：`filepath.EvalSymlinks` 解析后再次用 `filepath.Rel` 校验真实路径；写入 / 建目录 / 解压时解析最近已存在祖先，杜绝“父目录是外链”的逃逸。

12 个方法共用同一套 `ValidatePath / ResolveExisting / ResolveTarget / ResolveEntry`，没有旁路。拒绝一律返回 `E_INVALID` 并说明原因，错误信息不回显宿主绝对路径；根目录本身禁止删除、改权限、压缩或移动。

## 代码结构（单文件 ≤200 行）

| 文件 | 职责 |
| --- | --- |
| `internal/fileop/path.go` | 路径校验与符号链接解析（复用既有安全基础） |
| `internal/fileop/transfer.go` / `fs_transfer.go` | upload / download 契约、base64 解码、sha256、原子写与流式读 |
| `internal/fileop/archive_format.go` | 格式识别、条目名六层校验、符号链接目标校验 |
| `internal/fileop/archive_validate.go` | zip / tar 整包只读校验（不落盘） |
| `internal/fileop/archive.go` / `archive_write.go` | compress 编排、zip / tar.gz 写出、符号链接条目 |
| `internal/fileop/extract.go` / `archive_extract.go` | extract 编排、先校验后解压 |
| `internal/fileop/extract_write.go` | 目录 / 文件 / 链接落盘与临时文件 rename |
| `internal/fileop/attrs.go` | chmod：八进制解析、目录执行位校验、递归先校验后应用 |
| `internal/fileop/move.go` / `fs_move.go` / `move_copy.go` | move：rename、EXDEV 复制回退、目录树复制 |

## 测试

```bash
gofmt -l . && go vet ./... && go test -timeout 200s ./...
```

- `internal/fileop/*_test.go`：上传/下载（正常、超限、冲突、覆盖、穿越拒绝）、压缩/解压往返（zip 与 tar.gz）、恶意包（`../`、绝对路径、反斜杠、逃逸链接）整包拒绝且零残留、overwrite 两态、符号链接记 link 不跟随、chmod（0644→0600、递归、无 x 位报错）、move（改名/跨目录/冲突/覆盖/跨设备回退）
- `tests/fault/`：kill -9、超时、磁盘满、cgroup 超限四类注入，覆盖 8 个方法（含 upload/download/compress/chmod/move）
- `tests/e2e/`：用当前源码构建插件二进制，走真实 UDS 握手后跑 L2 全链路（压缩→解压→`diff -r`→上传/下载 sha256→chmod→move→恶意 zip 拒绝），`go test ./tests/e2e -v` 可看到每步 JSON 与 diff 退出码

## 四类故障注入

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
tar -tzf dist/file-0.2.0.tar.gz | head -1   # info.json
sha256sum dist/file-0.2.0.tar.gz             # 再跑一次 make dist 应完全一致
```

`dist` 使用确定性参数归档（`--sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner` + `gzip -9n`），重复构建 sha256 不变；`size_bytes` 由打包脚本回填，与实际包体偏差 ≤10%。

## 本机端到端

插件以内核客户端身份连接 UDS。没有内核时可用临时假内核完成握手后直接调用；`KORE_FILE_ROOT` 指向临时目录：

```bash
KORE_PLUGIN_SOCK=/tmp/kore-file-e2e.sock KORE_FILE_ROOT=/tmp/kore-file-e2e-init \
  ./bin/file
```

`tests/e2e` 已把这条链路固化成自动化用例：构建二进制 → 监听 UDS → 回写 `kore.hello` → 12 方法全链路 → `kore.shutdown` 优雅退出。
