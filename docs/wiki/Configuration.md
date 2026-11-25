# 文件管理插件 Wiki

语言：简体中文（本 Wiki 以简体中文为主，完整七语言说明见仓库 `docs/DETAIL*.md`）。

页面：[Home](https://github.com/kore-pl/kore-plugin-file/wiki/Home) · [Installation](https://github.com/kore-pl/kore-plugin-file/wiki/Installation) · [Configuration](https://github.com/kore-pl/kore-plugin-file/wiki/Configuration) · [Usage](https://github.com/kore-pl/kore-plugin-file/wiki/Usage) · [FAQ](https://github.com/kore-pl/kore-plugin-file/wiki/FAQ) · [Changelog](https://github.com/kore-pl/kore-plugin-file/wiki/Changelog)

## 配置契约

受控根目录由 `KORE_FILE_ROOT` 指定；回收站位置可经 `KORE_FILE_TRASH` 调整。路径穿越、NUL、符号链接逃逸、保留区 .trash 一律拒绝。

## 能力声明

`info.json` 的 `capabilities` 就是插件的全部授权面；方法级 `method_capabilities` 把只读与写/管理方法分开。声明多余能力会增加攻击路径，因此不要为方便而扩大授权。

## 运行约束

- 插件以独立进程运行，socket 由主壳创建、`chown kore-<id>`、权限 `0600`；只接受来自该 socket 的调用。
- lazy 插件空闲退出，不写 ticker / 轮询；超时用请求级 context。
- 日志写 stderr，`stdout` 只留给协议帧。

## 安全配置建议

漏洞不要开公开 issue，按仓库 [SECURITY.md](https://github.com/kore-pl/kore-plugin-file/blob/main/SECURITY.md) 私密披露。生产环境只授予实际需要的能力。
