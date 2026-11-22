# kore-plugin-file

文件管理插件（**骨架**）：覆盖文件在线编辑 / 上传 / 解压 / 权限的方法面。本仓只交付协议骨架与参数校验，**不真读写宿主文件、不内置任何工具二进制**（unzip/tar/chmod 等由使用者自行提供，裁决 37 / 提案 §2.1 批 1）。

## 协议面

- `kore.plugin.v1`，`lifecycle=lazy`，`depth_level=production`
- 能力声明：`system:read`
- 方法：`file.list` / `file.read` / `file.write`（同时接受裸 `list` / `read` / `write`）
- 握手的 stdout 留给协议帧，日志一律写 stderr

## 路径约束（骨架口径）

所有方法参数中的路径必须在配置根目录（`KORE_FILE_ROOT`，缺省 `/var/lib/kore/file`）之内：

- 拒绝绝对路径、空路径、`.` / `..` 路径段、反斜杠与 NUL
- 路径经 `path.Clean` 归一化后必须以根目录为前缀
- 根目录仅用于逻辑校验，**不执行 `os.ReadFile` / `os.WriteFile` / `os.MkdirAll`**

非法路径返回 `E_INVALID`；资源不存在返回 `E_NOTFOUND`。

## 四类故障注入

`tests/fault/` 四个文件分别注入 kill -9、超时、磁盘满、cgroup 超限，断言方法级返回结构化错误，且 `Error.Code` 与故障类别一一对应：

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
tar -tzf dist/file-0.1.0.tar.gz | head -1   # info.json
```

`dist` 使用确定性参数归档，重复构建 sha256 不变；`size_bytes` 由打包脚本回填，与实际包体偏差 ≤10%。
