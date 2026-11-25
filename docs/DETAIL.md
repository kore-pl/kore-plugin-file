# kore-plugin-file 使用说明

文件管理插件（L2+）：在受控根目录内提供 **19 个真实文件方法**——列表、读取、原子写入、删除（默认进回收站）、建目录、元信息、上传、下载、解压、压缩、改权限、移动、改名、复制、搜索、回收站列出/恢复/清空、批量操作。不内置 unzip/tar/chmod 等工具二进制，归档能力全部走 Go 标准库。

## 安装与兼容性

由 kore 内核安装发布的 tar.gz；仓库可独立构建（`make build` / `make dist`）。协议 `kore.plugin.v1`，`lifecycle=lazy`，`depth_level=production`，`requires_kore: >=0.1.0 <0.2.0`。插件级能力 `system:read` + `system:admin`；方法级能力把 6 个只读方法标 `system:read`，其余写/管理方法标 `system:admin`，与 `info.json` 逐条对应。方法名一律 `file.<verb>`，裸名仅历史兼容。

## 方法

| 方法 | 能力 | 入参 | 结果要点 |
| --- | --- | --- | --- |
| `file.list` | read | `{path}` | 条目含 name/path/is_dir/size/mode/mod_time/is_symlink/uid/gid；根目录隐藏 `.trash` |
| `file.read` | read | `{path, max_bytes?}` | UTF-8 content/size/truncated/is_binary；二进制不回 content |
| `file.stat` | read | `{path}` | size/mode/owner/mod_time/is_dir/is_symlink |
| `file.download` | read | `{path}` | filename/size_bytes/data_base64；超限提示改用目录打包 |
| `file.search` | read | `{path?,pattern,content?,ignore_case?,max_results?,max_file_bytes?}` | scanned/matched/skipped/truncated/entries（含 line/preview） |
| `file.trash.list` | read | `{limit?}` | count/total/skipped/truncated/entries，按删除时间倒序 |
| `file.write` | admin | `{path,content,create_if_missing?}` | 临时文件 + rename 原子替换 |
| `file.delete` | admin | `{path,recursive?,permanent?}` | 缺省进回收站；`permanent=true` 才直接删除 |
| `file.mkdir` | admin | `{path,mode?}` | 默认 0755 |
| `file.chmod` | admin | `{path,mode,recursive?}` | 整树先校验再统一应用 |
| `file.move` / `file.rename` / `file.copy` | admin | 源/目标或 path/name | 跨目录移动、仅改名、目录递归复制；符号链接按原目标重建 |
| `file.upload` / `file.uploadChunk` / `file.uploadStatus` | admin/read | `{path,filename,data_base64,offset?}` | 原子替换、断点续传（重复 offset 幂等） |
| `file.extract` / `file.compress` | admin | `{path,...}` | 走 Go 标准库，恶意 zip 拒绝 |
| `file.trash.restore` / `file.trash.purge` / `file.batch` | admin | 对应 id / 操作数组 | 恢复、不可恢复清空、批量逐项错误 |

## 安全边界

受控根目录由 `KORE_FILE_ROOT` 指定；路径穿越、NUL、符号链接逃逸、`.trash` 保留区一律拒绝。回收站位置可经 `KORE_FILE_TRASH` 调整。错误消息只带 errno，不泄露宿主绝对路径。四类故障注入覆盖 kill -9 / 超时 / 磁盘满 / cgroup 超限。

## 构建与验证

```bash
make check   # gofmt + go vet + go test -timeout 200s（含 race）
make build && make dist
```

## 安全与贡献

漏洞不要开公开 issue，按 [SECURITY.md](SECURITY.md) 私密披露；贡献见 [CONTRIBUTING.md](CONTRIBUTING.md)。MIT 许可。
