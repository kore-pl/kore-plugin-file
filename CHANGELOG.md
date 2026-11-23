# Changelog

## 0.3.2 (2026-11-23)

新增在线编辑的按字节范围读写（**非破坏性**）：

- **新增**：`file.readRange`（read）`{path, offset?, limit?}` → `{path, offset, size, returned, truncated, data_base64}`；默认窗口 256 KiB，单次上限 `MaxInlineTransferBytes`（≈5.95 MiB），大文件不整读
- **新增**：`file.writeRange`（admin）`{path, offset, data_base64, expected_sha256?}` → `{path, offset, written, size, sha256}`；覆盖写不截断（`offset == size` 为追加），返回写后整文件 sha256 供下一次乐观锁
- **并发语义（测试锁死）**：同一路径的读/写在本进程内按 path 互斥串行；跨客户端用 `expected_sha256` 乐观锁（整文件写前摘要），不匹配回 `E_CONFLICT` 且**不落盘**；`offset` 超过当前大小视为跳空，同样 `E_CONFLICT`（不自动补零，避免把编辑错误写成静默损坏）
- **安全**：路径复用 `ResolveExisting`（拒绝 `..` / 绝对路径 / 反斜杠 / 回收站目录 / 符号链接逃逸），路径穿越用例与两个新方法同测
- **不变**：`capabilities` / `triggers` / `requires_kore` / `lifecycle` 不变；`id@version` 换 sha 故 patch bump

## 0.3.1 (2026-11-23)

新增断点续传分块上传（**非破坏性**，既有 `file.upload` 不变）：

- **新增**：`file.uploadChunk`（admin）`{path, offset, data_base64, total_size?}` → `{path, received, total_size?, complete, replayed}`；按 `offset` 追加写入，单块上限沿用 `MaxInlineTransferBytes`（≈5.95 MiB）
- **新增**：`file.uploadStatus`（read）`{path}` → `{path, received, exists}`；客户端据此计算续传 offset，断线后从已收字节继续
- **重复 offset 语义（测试锁死）**：`offset == 已收字节` 为追加；`offset < 已收字节` 且重叠区字节一致视为重传——完全落在已收范围内返回 `replayed=true`（幂等，丢 ACK 不写坏文件），跨越端点只补追加未收部分；重叠区不一致或 `offset > 已收字节`（跳空）回 `E_CONFLICT`，提示先查 `file.uploadStatus`
- **安全**：路径复用 `ResolveTarget`（拒绝 `..` / 绝对路径 / 反斜杠 / 回收站目录 / 符号链接逃逸），路径穿越用例与两个新方法同测；`total_size` 越界直接 `E_INVALID`
- **不变**：`capabilities` / `triggers` / `requires_kore` / `lifecycle` 不变；`id@version` 换 sha 故 patch bump

## 0.3.0 (2026-11-23)

**破坏性语义变更（必读）**：`file.delete` 默认不再直接删除，改为移入配置根内的回收站 `<root>/.trash/`。

- **变更**：`file.delete` 缺省把目标移入回收站并返回 `trash_id`；结果新增 `permanent` 字段
- **兼容**：显式传 `permanent=true` 走旧语义（直接删除，不回收到站，结果 `permanent=true` 且无 `trash_id`）；`path` / `deleted` 字段保持不变，忽略新字段的旧调用方不会报错
- **迁移**：依赖「删完立即释放磁盘」的调用方改为 `permanent=true`，或定期 `file.trash.purge {"all":true}`；被删除对象在 `purge` 前仍占用磁盘
- **不变**：非空目录仍需 `recursive=true`、根目录禁止删除、符号链接只删链接本身、路径穿越与链接逃逸一律拒绝

新增方法：

- feat: `file.rename`——同目录改名（`path` + 单段 `name`），与 `file.move` 分工明确：rename 不能换目录，`name` 拒绝 `/`、`\`、`..` 与保留名 `.trash`；冲突策略与 move 一致（`overwrite`）
- feat: `file.copy`——文件 / 目录递归 / 符号链接复制，保留权限位、链接按原目标重建；覆盖策略显式（同类型覆盖、非空目录不合并、类型不符 `E_CONFLICT`）；临时项 + rename 落位，失败不留半副本；特殊文件 `E_UNSUPPORTED`
- feat: `file.search`——文件名 glob 必选，`content` 可选（名称命中后再做内容过滤）；内容搜索只读常规文件且 ≤ `max_file_bytes`（默认 1 MiB / 上限 8 MiB），含 NUL 的二进制、超体积、符号链接与特殊文件计入 `skipped`，不把大文件读进内存；`max_results` 默认 200 / 上限 1000，单次遍历上限 20 万条，命中或遍历上限触发 `truncated`
- feat: 回收站三方法——`file.trash.list`（按时间倒序、损坏条目计入 `skipped`）、`file.trash.restore`（按原相对路径恢复，父目录必须仍存在且仍在根内，恢复目标冲突需 `overwrite`）、`file.trash.purge`（`id` 或 `all=true`，不可恢复，空参数直接拒绝）
- feat: `file.batch`——批量 `delete` / `move` / `copy`，最多 200 条；**部分失败逐项返回**（`ok` + `error{code,message}`，`succeeded`/`failed` 汇总），单条失败不影响其余，只有结构性错误才整体失败
- feat: 方法级 `method_capabilities`（§3.5 #12）——6 个只读方法 `system:read`、13 个写/管理方法 `system:admin`，与分发表逐条一致；新增 CI 断言镜像内核 `registry/validate.go` 的键格式 / 值域 / 条数校验

安全：

- feat: 保留路径 `.trash` 统一拦截——`ValidatePath` 拒绝对回收站首段的任何普通方法访问（list/read/write/delete/mkdir/stat/upload/download/extract/compress/chmod/move/rename/copy/search/batch），根目录列表与搜索结果不展示它
- feat: 回收站根目录自身校验——必须是配置根真实路径内的真实目录；被替换为符号链接或同名文件时拒绝（否则删除会把文件搬到根外）
- feat: 回收站 id 限定 `^[0-9]{19}-[0-9a-f]{8}$`，恢复路径复用 `resolveDestination`（父目录存在 + 根内），meta 损坏按 `E_INTERNAL` 报错、列表只计入 `skipped`

测试与构建：

- test: 新增 rename / copy / search / 回收站 / 批量 / L3 安全（穿越 + 符号链接逃逸 + `.trash` 保留 + 恶意 meta）用例；故障注入扩到 16 个方法（13 个矩阵方法 + batch 逐项错误 + trash.restore / trash.purge 的四类故障映射）；e2e 增加 L3 全链路
- docs: README 补 19 方法契约、rename vs move、copy 覆盖策略、search 边界、回收站语义变更与迁移指引、批量部分失败语义
- build: 版本 0.3.0（新增 7 个方法 → minor），`info.json` / `manifest.json` 同步

## 0.2.1 (2026-11-23)

- build: 位级可复现打包（stage chmod + tar --mode 归一、size_bytes 收敛），二进制改嵌 manifest.json，消除同版本 sha 漂移

## 0.2.0 (2026-11-23)

- feat: `file.upload` / `file.download`——base64 传输、原子落盘、sha256 回执；同名冲突 `E_CONFLICT`、`overwrite=true` 覆盖并继承权限位；下载超限提示“文件过大，请用目录打包下载”
- feat: 传输上限口径显式化——方法契约 32 MiB，但受协议单帧 8 MiB 限制，单次内联实际 ≈5.95 MiB（`MaxInlineTransferBytes`），超限错误写明单帧/base64/契约三层口径，分片留 L3
- feat: `file.extract`——zip / tar / tar.gz / tgz；**先整包校验后落盘**，恶意包零残留；条目名六层校验（非空、≤4096、无 NUL/反斜杠、非绝对路径、无 `..` 段、Clean 后合法）+ 类型白名单 + 2 GiB 单条目上限 + 符号链接目标归属 + 逐条目 `ResolveTarget`；`overwrite=false` 记 `skipped[]`
- feat: `file.compress`——zip / tar.gz；多路径递归、保留根相对路径、目录条目与空目录、符号链接记 link 不跟随；归档不得写进被压缩目录内部；同目录临时文件 + fsync + rename
- feat: `file.chmod`——八进制 mode、递归先整树校验再统一应用；目录缺执行位直接 `E_INVALID` 且不改任何权限；禁止改配置根目录自身
- feat: `file.move`——同根重命名/移动；目标冲突 `E_CONFLICT`、`overwrite=true` 覆盖（目录只允许空目录）；跨文件系统 EXDEV 回退“复制到临时项 + rename + 删除源”，结果 `method=copy+delete`
- refactor: 执行面 `Executor` 扩展 6 个方法；新方法按功能拆文件（`transfer` / `archive_*` / `extract*` / `attrs` / `move*`），单文件 ≤200 行；归档与移动的平台差异收在 `move_unix.go` / `move_other.go`
- test: 单元测试覆盖 6 个新方法的正常/边界/安全拒绝；恶意 zip 与 tar（`../`、绝对路径、反斜杠、逃逸符号链接）整包拒绝且目标目录不创建
- test: 故障注入从 3 方法扩到 8 方法，四类故障码保持不变
- test: 新增 `tests/e2e`——当前源码构建二进制 + 真实 UDS 握手，压缩→解压→`diff -r`→上传下载 sha256→chmod→move→恶意 zip 拒绝，全部自动化
- docs: README 补齐 12 方法契约、传输口径、解压六层防穿越与代码结构；`info.json` 版本 0.2.0

## 0.1.0 (2026-11-22)

- feat: 新建 file 插件骨架，复用 kore.plugin.v1 握手与会话循环
- feat: file.list / file.read / file.write / file.delete / file.mkdir / file.stat 六方法真实落盘，替换占位执行面
- feat: 路径安全双校验——`filepath.Clean` + `filepath.Rel` 词法判定，`filepath.EvalSymlinks` 后二次校验；拒绝 `..` 穿越、绝对路径、反斜杠、NUL 与指向根目录外的符号链接，错误信息不回显宿主路径
- feat: 读写上限——read 默认 1 MiB（`max_bytes` 可调到 8 MiB），write 8 MiB；二进制（含 NUL）只回元信息
- feat: file.write 采用同目录临时文件 + fsync + rename 原子替换，失败清理临时文件、目标保持原内容；继承已有文件权限位
- feat: file.delete 区分非空目录（`recursive=false` 回 E_CONFLICT）与递归删除；file.mkdir 默认 0755、已存在回 E_CONFLICT
- feat: file.stat 返回 size / mode / owner(uid,gid) / mod_time / is_dir / is_symlink
- test: 单元测试在 `t.TempDir()` 内做真实文件操作，覆盖 list / read / write / delete / mkdir / stat、原子性失败与全部安全拒绝路径
- test: 四类故障注入测试（kill -9 / 超时 / 磁盘满 / cgroup 超限），验证方法级错误返回
- build: 确定性打包（--sort=name --mtime=@0 --owner=0 --numeric-owner + gzip -9n）
- chore: 能力声明补 `system:admin`（真实写盘），配置根目录缺省改为 `/tmp/kore-file-root` 便于本机验证，`resources.memory_mb` 6→8 以容纳真实读写缓冲
