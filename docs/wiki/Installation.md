# 文件管理插件 Wiki

语言：简体中文（本 Wiki 以简体中文为主，完整七语言说明见仓库 `docs/DETAIL*.md`）。

页面：[Home](https://github.com/kore-pl/kore-plugin-file/wiki/Home) · [Installation](https://github.com/kore-pl/kore-plugin-file/wiki/Installation) · [Configuration](https://github.com/kore-pl/kore-plugin-file/wiki/Configuration) · [Usage](https://github.com/kore-pl/kore-plugin-file/wiki/Usage) · [FAQ](https://github.com/kore-pl/kore-plugin-file/wiki/FAQ) · [Changelog](https://github.com/kore-pl/kore-plugin-file/wiki/Changelog)

## 前置条件

- kore 内核（协议 `kore.plugin.v1`，`requires_kore: >=0.1.0 <0.2.0`）。
- Linux 为主支持平台；其它平台的真实实现范围见仓库 README。
- 仅标准库 + `github.com/pengtang1024/kore/pkg/pluginapi`，无第三方运行时依赖。

## 安装

由 kore 内核的插件安装器安装本仓库 Release 发布的 tar.gz（包内首条目为 `info.json`）。安装器会按 `info.json` 的 `id`、`capabilities`、`lifecycle` 登记插件。

## 源码构建（可选）

```bash
git clone https://github.com/kore-pl/kore-plugin-file.git
cd kore-plugin-file
make check    # gofmt + go vet + go test -timeout 200s（含 race）
make build    # 静态单二进制
make dist     # 确定性 tar.gz + .sha256
```

构建基线：`CGO_ENABLED=0`、`GOWORK=off`、`GOAMD64=v1`。不要提交 `go.work` 或跨仓 `replace`。

## 升级与卸载

升级时安装新版本 tar.gz；`id@version` 与包 sha256 由 [kore-plugins](https://github.com/kore-pl/kore-plugins) 索引固定。卸载按内核插件管理流程执行。
