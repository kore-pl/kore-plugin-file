# Changelog

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
