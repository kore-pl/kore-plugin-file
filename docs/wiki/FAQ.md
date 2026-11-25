# 文件管理插件 Wiki

语言：简体中文（本 Wiki 以简体中文为主，完整七语言说明见仓库 `docs/DETAIL*.md`）。

页面：[Home](https://github.com/kore-pl/kore-plugin-file/wiki/Home) · [Installation](https://github.com/kore-pl/kore-plugin-file/wiki/Installation) · [Configuration](https://github.com/kore-pl/kore-plugin-file/wiki/Configuration) · [Usage](https://github.com/kore-pl/kore-plugin-file/wiki/Usage) · [FAQ](https://github.com/kore-pl/kore-plugin-file/wiki/FAQ) · [Changelog](https://github.com/kore-pl/kore-plugin-file/wiki/Changelog)

## 常见问题

### 删除的文件能恢复吗？

默认进回收站，可用 file.trash.list / restore 恢复；file.trash.purge 不可恢复。

### 如何下载大文件？

file.download 超限会提示改用目录打包；编辑器场景用 file.readRange 分页读取。

### 支持断点续传吗？

支持：file.uploadChunk 按 offset 写入，重复 offset 幂等，file.uploadStatus 查询进度。

### 错误消息会泄露路径吗？

不会：只带 errno 与处理建议，绝不拼宿主绝对路径。

### 去哪里报 bug / 提需求？

在仓库开 issue；安全漏洞不要开公开 issue，按 [SECURITY.md](https://github.com/kore-pl/kore-plugin-file/blob/main/SECURITY.md) 私密披露。
