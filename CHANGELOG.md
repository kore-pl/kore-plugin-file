# Changelog

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
