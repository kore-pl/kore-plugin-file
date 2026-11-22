# Changelog

## 0.1.0 (2026-11-22)

- feat: 新建 file 插件骨架，复用 kore.plugin.v1 握手与会话循环
- feat: file.list / file.read / file.write 方法级参数校验与结构化结果，路径限制在配置根目录内，不真读写宿主文件
- feat: 四类故障注入测试（kill -9 / 超时 / 磁盘满 / cgroup 超限），验证方法级错误返回
- build: 确定性打包（--sort=name --mtime=@0 --owner=0 --numeric-owner + gzip -9n）
