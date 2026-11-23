# kore-plugin-file

文件管理插件（**L2+**）：在受控根目录内提供 **19 个真实文件方法**——列表、读取、原子写入、删除（默认进回收站）、建目录、元信息、上传、下载、解压、压缩、改权限、移动、改名、复制、搜索、回收站列出/恢复/清空、批量操作。不内置 unzip/tar/chmod 等工具二进制（裁决 37），归档能力全部走 Go 标准库。

## 协议面

- `kore.plugin.v1`，`lifecycle=lazy`，`depth_level=production`
- 插件级能力：`system:read` + `system:admin`（真实写盘需要管理能力）
- **方法级能力**（`method_capabilities`，§3.5 #12）：只读 6 项声明 `system:read`，写 / 管理 13 项声明 `system:admin`，与下表逐条对应；`internal/fileop/capability_test.go` 会在 CI 里比对 `info.json` 与分发表，防止声明漂移
- 方法名一律 `file.<verb>`；裸名（`list` / `read` / `write` / `delete` / `mkdir` / `stat`）仅为历史兼容，L2 / L3 新方法必须带 `file.` 前缀

| 方法 | 能力 | 入参 | 结果要点 |
| --- | --- | --- | --- |
| `file.list` | read | `{path}` | `entries[]`：`name` / `path` / `is_dir` / `size` / `mode` / `mod_time`(RFC3339) / `is_symlink`；根目录不展示保留的 `.trash` |
| `file.read` | read | `{path, max_bytes?}` | `content`（UTF-8）/ `size` / `truncated` / `is_binary`；含 NUL 判二进制，不回 `content` |
| `file.stat` | read | `{path}` | `size` / `mode` / `owner{uid,gid}` / `mod_time` / `is_dir` / `is_symlink` |
| `file.download` | read | `{path}` | `filename` / `size_bytes` / `data_base64`；超限回 `E_INVALID` 并提示“文件过大，请用目录打包下载” |
| `file.search` | read | `{path?, pattern, content?, ignore_case?, max_results?, max_file_bytes?}` | `scanned` / `matched` / `skipped` / `truncated` / `entries[]`（含 `line` 与 `preview`） |
| `file.trash.list` | read | `{limit?}` | `count` / `total` / `skipped` / `truncated` / `entries[]`（`id` / `path` / `kind` / `size` / `deleted_at`），按删除时间倒序 |
| `file.write` | admin | `{path, content, create_if_missing?}` | `size` / `mod_time` / `created`；临时文件 + `rename` 原子替换 |
| `file.delete` | admin | `{path, recursive?, permanent?}` | `deleted` / `permanent` / `trash_id`；**缺省进回收站**，`permanent=true` 才是旧语义的直接删除 |
| `file.mkdir` | admin | `{path, mode?}` | `mode`（默认 `0755`，八进制字符串）/ `created` |
| `file.chmod` | admin | `{path, mode, recursive?}` | `path` / `mode` / `changed`；目录缺执行位回 `E_INVALID`，递归先整树校验再统一应用 |
| `file.move` | admin | `{src, dst, overwrite?}` | `from` / `to` / `moved` / `method`；可跨目录；`method=rename` 或跨文件系统回退 `copy+delete` |
| `file.rename` | admin | `{path, name, overwrite?}` | `from` / `to` / `renamed` / `method`；**只能改名不能换目录**，`name` 必须是单段文件名 |
| `file.copy` | admin | `{src, dst, overwrite?}` | `from` / `to` / `copied` / `kind` / `entries` / `bytes`；目录递归、链接按原目标重建 |
| `file.upload` | admin | `{path, filename, data_base64, overwrite?}` | `path` / `filename` / `size` / `sha256` / `created`；原子替换并继承已有权限位 |
| `file.compress` | admin | `{paths[], archive_path, format?}` | `archive_path` / `size_bytes` / `entries` / `format`（`zip` 缺省，或 `tar.gz`）；递归目录、保留相对路径、不跟随符号链接 |
| `file.extract` | admin | `{archive_path, dest_dir, overwrite?}` | `extracted` / `skipped[]` / `bytes` / `format`；支持 zip / tar / tar.gz / tgz；先整包校验后落盘 |
| `file.trash.restore` | admin | `{id, overwrite?}` | `id` / `path` / `restored`；按原相对路径恢复，父目录必须仍存在且仍在根内 |
| `file.trash.purge` | admin | `{id}` 或 `{all:true}` | `purged` / `all`；**不可恢复**，空参数直接 `E_INVALID` |
| `file.batch` | admin | `{op, items[]}` | `op` ∈ `delete` / `move` / `copy`；`total` / `succeeded` / `failed` / `items[]`，逐项 `ok` 与 `error{code,message}` |

### rename 与 move 的区别（刻意保留两个动词）

- `file.move`：`src` → `dst`，**可以换目录**，目标是完整相对路径；
- `file.rename`：`path` + `name`，**只能同目录改名**，`name` 不允许出现 `/`、`\`、`..`，也不允许 `".trash"` 这类保留名。
  想跨目录请用 `file.move`，错误信息会直接给出这条指引。两者共用同一套目标冲突策略（`overwrite`）与底层 `rename` 回退逻辑。

### copy 的覆盖策略（显式，不隐式合并）

- 目标不存在：正常复制（文件 / 目录树 / 符号链接）；
- 目标存在且 `overwrite=false`：`E_CONFLICT`；
- 目标存在且 `overwrite=true`：只允许**同类型**覆盖——文件覆盖文件、空目录覆盖空目录；文件覆盖目录、目录覆盖文件、覆盖非空目录一律 `E_CONFLICT`（不做 merge）；
- 复制先落到目标目录的临时项再 `rename` 就位，失败不留半个副本；特殊文件（设备 / FIFO）回 `E_UNSUPPORTED`。

### search 的边界（内容搜索不把大文件读进内存）

- `pattern` 是必填的文件名 glob（`path.Match` 语法）；含 `/` 时按根相对路径匹配，`ignore_case` 可大小写折叠；
- `content` 非空时是「名称 glob 之上的二次过滤」：名字不匹配的文件根本不会打开；
- 内容搜索只读**常规文件**且 `size ≤ max_file_bytes`（缺省 1 MiB，硬上限 8 MiB）；含 NUL 的二进制、超体积文件、符号链接与特殊文件计入 `skipped` 而不算命中；
- `max_results` 缺省 200、上限 1000；单次最多遍历 20 万条，命中上限或遍历上限触发时 `truncated=true`；
- 不跟随符号链接，跳过保留的 `.trash`，搜索根必须是目录。

### 回收站语义（0.3.0 的破坏性变更，必须知道）

`file.delete` **默认不再直接删除**：目标被移入 `<KORE_FILE_ROOT>/.trash/entries/<id>/`（`meta.json` 记录原相对路径、类型、大小、删除时间，`data` 是被移入对象），可用 `file.trash.list` 查看、`file.trash.restore` 按原路径恢复、`file.trash.purge` 永久清除。

- **保留旧语义**：显式传 `permanent=true`，行为与 0.2.x 完全一致（直接删除、不进回收站），结果里 `permanent=true` 且没有 `trash_id`；
- **不变的部分**：非空目录仍需 `recursive=true`（否则 `E_CONFLICT`）、配置根目录仍然禁止删除、符号链接仍然只删链接本身、路径穿越与链接逃逸仍然一律拒绝；
- **迁移指引**：依赖「删完立即释放磁盘」的调用方请改成 `permanent=true`，或定期 `file.trash.purge {"all":true}`；调用方若忽略新增字段也能继续工作（`path` / `deleted` 仍在）；
- **回收站位置与保护**：固定为配置根内的 `.trash`（与数据同文件系统，移动是原子 rename）。普通方法一律拒绝访问该目录（`E_INVALID`），根目录列表与搜索结果也不展示它；若 `.trash` 被替换成符号链接或同名文件，删除/列回收站都会 `E_INVALID` 而不是把文件搬出根目录。

### 批量操作的部分失败语义

`file.batch` 一次只做一种 `op`（`delete` / `move` / `copy`），最多 200 条。**单条失败只影响该条**：整体仍返回成功帧，逐项 `ok` 与 `error{code,message}` 交代原因，`succeeded` / `failed` 汇总。只有结构性错误（`op` 未知、`items` 为空或超限）才让整次调用失败。批量 DELETE 同样默认进回收站，逐项结果里的 `trash_id` 可直接用于恢复。

## 传输上限（口径必须说清）

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
- **逐条目路径归属**：每个条目落盘前再走一次与 `file.read`/`file.write` 完全相同的 `ResolveTarget`，劫持父目录的符号链接同样被拒；含保留 `.trash` 段的条目也在此被拒

解压采用**先整包校验、后落盘**两遍：校验不通过时目标目录不会被创建，恶意包零残留。

## 路径安全约束

所有路径限制在配置根目录（`KORE_FILE_ROOT`，缺省 `/tmp/kore-file-root`）之内，两层校验 + 一条保留路径：

1. **词法校验**：拒绝空路径、绝对路径、`..` 段、反斜杠与 NUL，以及首段命中保留目录 `.trash` 的路径；`filepath.Clean` 后用 `filepath.Rel` 判定仍在根内。
2. **符号链接二次校验**：`filepath.EvalSymlinks` 解析后再次用 `filepath.Rel` 校验真实路径；写入 / 建目录 / 解压 / 恢复回收站时解析最近已存在祖先，杜绝“父目录是外链”的逃逸。
3. **回收站保留**：`.trash` 只能由 `file.trash.*` 访问，且其自身必须是配置根内的真实目录（不是符号链接），否则拒绝——回收站不能成为新的越界口。

19 个方法共用同一套 `ValidatePath / ResolveExisting / ResolveTarget / ResolveEntry` 与 `resolveDestination`，没有旁路。拒绝一律返回 `E_INVALID` 并说明原因，错误信息不回显宿主绝对路径；根目录本身禁止删除、改权限、压缩或移动。

## 代码结构（单文件 ≤200 行）

| 文件 | 职责 |
| --- | --- |
| `internal/fileop/path.go` | 路径校验、保留目录拦截与符号链接解析（复用既有安全基础） |
| `internal/fileop/transfer.go` / `fs_transfer.go` | upload / download 契约、base64 解码、sha256、原子写与流式读 |
| `internal/fileop/archive_format.go` / `archive_validate.go` | 格式识别、条目名六层校验、zip / tar 整包只读校验 |
| `internal/fileop/archive.go` / `archive_write.go` | compress 编排、zip / tar.gz 写出、符号链接条目 |
| `internal/fileop/extract.go` / `archive_extract.go` / `extract_write.go` | extract 编排、先校验后解压 |
| `internal/fileop/attrs.go` | chmod：八进制解析、目录执行位校验、递归先校验后应用 |
| `internal/fileop/move.go` / `fs_move.go` / `move_copy.go` | move：rename、EXDEV 复制回退、目录树复制 |
| `internal/fileop/rename.go` | rename：同目录改名的单段名校验与目标推导 |
| `internal/fileop/copy.go` / `fs_copy.go` | copy：覆盖策略、临时项 + rename、复制统计 |
| `internal/fileop/search.go` / `fs_search.go` | search：glob / 内容过滤、阈值校验、有界遍历与预览脱敏 |
| `internal/fileop/trash.go` / `trash_types.go` / `trash_base.go` | trash.* 方法面、显式结构、回收站根目录安全校验 |
| `internal/fileop/fs_trash.go` / `fs_trash_restore.go` | 回收站落盘：条目目录 + meta.json、列出、恢复、清除 |
| `internal/fileop/batch.go` | batch：结构校验、逐项复用单方法处理函数、部分失败汇总 |

## 测试

```bash
gofmt -l . && go vet ./... && go test -timeout 200s ./...
```

- `internal/fileop/*_test.go`：既有 12 方法的正常 / 边界 / 安全用例，新增 rename（成功、目录、冲突/覆盖、路径语义拒绝、链接逃逸）、copy（文件/目录/链接、覆盖策略、自身子树、链接逃逸）、search（名/内容两态、体积上限与二进制跳过、结果截断、回收站跳过、参数校验）、回收站（删→列→恢复→内容一致、目录树往返、恢复冲突、purge 不可恢复、损坏 meta）、批量（部分失败、回收站 id、move/copy、结构性错误）、方法级能力声明与内核校验镜像
- `tests/fault/`：kill -9、超时、磁盘满、cgroup 超限四类注入，13 个方法的注入矩阵（含 search / copy / rename / delete / trash.list），并对 batch 逐项错误与 trash.restore / trash.purge 单独覆盖（合计 16 个方法）
- `tests/e2e/`：用当前源码构建插件二进制，走真实 UDS 握手；L2 链路（压缩→解压→`diff -r`→上传/下载 sha256→chmod→move→恶意 zip 拒绝）+ L3 链路（rename→copy→内容 search→删进回收站→列表→恢复→批量部分失败→purge→穿越与 `.trash` 拒绝）

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
tar -tzf dist/file-0.3.0.tar.gz | head -1   # info.json
sha256sum dist/file-0.3.0.tar.gz             # 再跑一次 make dist 应完全一致
```

`dist` 使用确定性参数归档（`--sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner` + `gzip -9n`），重复构建 sha256 不变；`size_bytes` 由打包脚本回填，与实际包体偏差 ≤2 字节。

## 本机端到端

插件以内核客户端身份连接 UDS。没有内核时可用临时假内核完成握手后直接调用；`KORE_FILE_ROOT` 指向临时目录：

```bash
KORE_PLUGIN_SOCK=/tmp/kore-file-e2e.sock KORE_FILE_ROOT=/tmp/kore-file-e2e-init \
  ./bin/file
```

`tests/e2e` 已把这条链路固化成自动化用例：构建二进制 → 监听 UDS → 回写 `kore.hello` → 19 方法中的代表链路（L2 + L3）→ `kore.shutdown` 优雅退出。

<!-- oss:begin -->
## 开源与贡献

[![CI](https://github.com/kore-pl/kore-plugin-file/actions/workflows/ci.yml/badge.svg)](https://github.com/kore-pl/kore-plugin-file/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go 1.26](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](go.mod)

本仓库是 **kore** 的 `文件管理插件`（id `file`），独立仓库、独立版本线、独立 CI（少触发：push main / tag / PR / 手动）。内核与协议见 [kore](https://github.com/kore-pl/kore)。

- **许可**：MIT（inbound = outbound，不要求 CLA）；提交用 `git commit -s` 签 DCO。
- **贡献**：[CONTRIBUTING.md](CONTRIBUTING.md)（构建、门禁、发版、边界）。
- **安全**：[SECURITY.md](SECURITY.md)——漏洞不要开公开 issue。
- **行为准则**：[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)。
- **兼容性**：`protocol: kore.plugin.v1` · `requires_kore: >=0.1.0 <0.2.0` · `lifecycle: lazy` · 深度等级 `production`。
- **验证方式**：本地 `make check` 优先（CI 只在 push main / tag / PR / 手动跑；发布前 `make dist` 连跑两次核对 sha 一致）。
- **发布产物**：`make dist` → `dist/file-0.3.0.tar.gz` + `.sha256`（位级可复现，连跑两次 sha 相同）。
- **相关仓库**：[索引 kore-plugins](https://github.com/kore-pl/kore-plugins) · [协议文档](https://github.com/kore-pl/kore/blob/main/docs/PLUGIN-PROTOCOL.md) · [SDK 指南](https://github.com/kore-pl/kore/blob/main/docs/PLUGIN-SDK.md)

<!-- oss:end -->
