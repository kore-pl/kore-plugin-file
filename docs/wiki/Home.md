# 文件管理插件 Wiki

语言：简体中文（本 Wiki 以简体中文为主，完整七语言说明见仓库 `docs/DETAIL*.md`）。

页面：[Home](https://github.com/kore-pl/kore-plugin-file/wiki/Home) · [Installation](https://github.com/kore-pl/kore-plugin-file/wiki/Installation) · [Configuration](https://github.com/kore-pl/kore-plugin-file/wiki/Configuration) · [Usage](https://github.com/kore-pl/kore-plugin-file/wiki/Usage) · [FAQ](https://github.com/kore-pl/kore-plugin-file/wiki/FAQ) · [Changelog](https://github.com/kore-pl/kore-plugin-file/wiki/Changelog)

## 这是什么

文件管理插件（id `file`）是 kore 的受控根目录内的 L2+ 文件管理：19 个真实文件方法；不内置 unzip/tar/chmod 等工具二进制，归档走 Go 标准库。

## 核心能力

- 列表、读取、原子写入、删除（默认回收站）、建目录、元信息
- 上传/下载、分片续传（重复 offset 幂等）、readRange 分页读取
- 压缩/解压（恶意 zip 拒绝）、chmod、move/rename/copy
- 内容搜索、回收站列出/恢复/清空、批量逐项错误
- 方法级能力声明 6 read + 13 admin，与 info.json 双向校验
- 四类故障注入：kill -9 / 超时 / 磁盘满 / cgroup 超限

协议 `kore.plugin.v1`，`lifecycle=lazy`，深度等级见仓库 `info.json`。内核、协议与 SDK 文档见 [kore](https://github.com/kore-pl/kore)。

## 快速开始

1. 安装：见 [Installation](https://github.com/kore-pl/kore-plugin-file/wiki/Installation)。
2. 配置：见 [Configuration](https://github.com/kore-pl/kore-plugin-file/wiki/Configuration)。
3. 调用：见 [Usage](https://github.com/kore-pl/kore-plugin-file/wiki/Usage) 与仓库 README 的方法表。
4. 遇到问题：见 [FAQ](https://github.com/kore-pl/kore-plugin-file/wiki/FAQ)。
